package newapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hachimodockplus/internal/config"
)

const ramFixture = `{"access_key_id":"LTAItestkey","access_key_secret":"test-ram-secret"}`
const quotaFixture = `{"data":{"DataV2":{"data":{"data":{"per1MonthPercentage":0.25}}}}}`

type authTransport func(*http.Request) (*http.Response, error)

func (f authTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func authResponse(status int, body string) (*http.Response, error) {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
}

func ramProvider(t *testing.T) ProviderConfig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ram.key")
	if err := os.WriteFile(path, []byte(ramFixture), 0600); err != nil {
		t.Fatal(err)
	}
	return ProviderConfig{Kind: "aliyun-token-plan", Auth: "access-key", Service: config.Service{URL: "https://bailian-cs.console.aliyun.com", KeyFile: path}}
}

func TestACS3Signature(t *testing.T) {
	headers := map[string]string{
		"host": "modelstudio.cn-beijing.aliyuncs.com", "content-type": "application/json",
		"x-acs-action": "GenerateCLIAccessToken", "x-acs-version": "2026-02-10",
		"x-acs-content-sha256": sha256Hex(""), "x-acs-date": "2026-11-21T12:00:00Z",
		"x-acs-signature-nonce": "0123456789abcdef0123456789abcdef",
	}
	signAliyunRequest("/modelstudio/cli/generateAccessToken", headers, aliyunAccessKey{ID: "LTAItestkey", Secret: "test-ram-secret"})
	// Independently calculated with Python hashlib/hmac from the documented ACS3 canonical request.
	expected := "ACS3-HMAC-SHA256 Credential=LTAItestkey,SignedHeaders=content-type;host;x-acs-action;x-acs-content-sha256;x-acs-date;x-acs-signature-nonce;x-acs-version,Signature=74eb64448453803eea696b06dfe051f5058bd1d20bd5136a9fa160d65c5a86c3"
	if headers["Authorization"] != expected {
		t.Fatal("ACS3 signature does not match reference vector")
	}
}

func TestAliyunSingaporeSTSRequest(t *testing.T) {
	c := clientFor(t, func(http.ResponseWriter, *http.Request) {})
	p := ramProvider(t)
	p.Region = "ap-southeast-1"
	p.URL = "https://bailian-singapore-cs.alibabacloud.com"
	if err := os.WriteFile(p.KeyFile, []byte(`{"access_key_id":"STS.testkey","access_key_secret":"test-secret","security_token":"test-sts-token"}`), 0600); err != nil {
		t.Fatal(err)
	}
	c.http.Transport = authTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/modelstudio/cli/generateAccessToken" {
			if r.URL.Host != "modelstudio.ap-southeast-1.aliyuncs.com" || r.Header.Get("X-Acs-Security-Token") != "test-sts-token" || !strings.Contains(r.Header.Get("Authorization"), ";x-acs-security-token;") {
				t.Error("incorrect STS signing or token region")
			}
			return authResponse(200, `{"cliAccessToken":"singapore-token"}`)
		}
		if r.URL.Host != "bailian-singapore-cs.alibabacloud.com" || r.URL.Query().Get("action") != "IntlBroadScopeAspnGateway" {
			t.Error("incorrect regional quota gateway")
		}
		return authResponse(200, quotaFixture)
	})
	if b := c.provider(context.Background(), p); b.Error != "" {
		t.Fatal(b.Error)
	}
}

func TestAliyunCacheRefreshAndCredentialRotation(t *testing.T) {
	c := clientFor(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected NewAPI request") })
	p := ramProvider(t)
	issued, queries := 0, 0
	expired := ""
	c.http.Transport = authTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/modelstudio/cli/generateAccessToken" {
			issued++
			if r.URL.Host != "modelstudio.cn-beijing.aliyuncs.com" || r.Method != "POST" || r.Header.Get("X-Acs-Action") != "GenerateCLIAccessToken" || !strings.HasPrefix(r.Header.Get("Authorization"), "ACS3-HMAC-SHA256 Credential=LTAItestkey,") {
				t.Error("incorrect signed token request")
			}
			return authResponse(200, fmt.Sprintf(`{"cliAccessToken":"token-%d"}`, issued))
		}
		queries++
		if r.Header.Get("Authorization") == "Bearer "+expired {
			return authResponse(200, `{"data":{"success":false,"errorCode":"BailianGateway.Login.NotLogined"}}`)
		}
		return authResponse(200, quotaFixture)
	})
	check := func(wantIssued, wantQueries int) {
		t.Helper()
		b := c.provider(context.Background(), p)
		if b.Error != "" || b.Value != "75.0%" || issued != wantIssued || queries != wantQueries {
			t.Fatalf("balance=%+v issued=%d queries=%d", b, issued, queries)
		}
	}
	check(1, 1)
	check(1, 2) // Reuse the token on the next dashboard refresh.
	expired = "token-1"
	check(2, 4) // A cached expired token causes one exchange and one quota retry.
	check(2, 5)
	if err := os.WriteFile(p.KeyFile, []byte(strings.ReplaceAll(ramFixture, "test-ram-secret", "rotated-secret")), 0600); err != nil {
		t.Fatal(err)
	}
	check(3, 6)
}

func TestAliyunRetryLimitsAndSafeErrors(t *testing.T) {
	for _, mode := range []string{"expired", "http401", "http403", "team", "permission", "token403", "invalidToken", "badJSON"} {
		t.Run(mode, func(t *testing.T) {
			c := clientFor(t, func(http.ResponseWriter, *http.Request) {})
			p := ramProvider(t)
			issued, queries := 0, 0
			failing := false
			c.http.Transport = authTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/modelstudio/cli/generateAccessToken" {
					issued++
					if failing && mode == "token403" {
						return authResponse(403, `{"Message":"test-ram-secret"}`)
					}
					if failing && mode == "invalidToken" {
						return authResponse(200, `{"cliAccessToken":""}`)
					}
					return authResponse(200, `{"cliAccessToken":"private-console-token"}`)
				}
				queries++
				if !failing {
					return authResponse(200, quotaFixture)
				}
				switch mode {
				case "http401":
					return authResponse(401, "private-console-token")
				case "http403":
					return authResponse(403, "test-ram-secret")
				case "team":
					return authResponse(200, `{"data":{"success":false,"errorCode":"BailianGateway.Team.NotAuthorised","errorMsg":"test-ram-secret"}}`)
				case "permission":
					return authResponse(200, `{"data":{"success":false,"errorCode":"BailianGateway.AuthorityPolicies.NoPermission","errorMsg":"test-ram-secret"}}`)
				case "badJSON":
					return authResponse(200, "test-ram-secret")
				default:
					return authResponse(200, `{"data":{"success":false,"errorCode":"BailianGateway.Login.NotLogined"}}`)
				}
			})
			if b := c.provider(context.Background(), p); b.Error != "" {
				t.Fatal(b.Error)
			}
			failing = true
			b := c.provider(context.Background(), p)
			if b.Error == "" || strings.Contains(b.Error, "test-ram-secret") || strings.Contains(b.Error, "private-console-token") {
				t.Fatal("missing or unsafe error", b.Error)
			}
			wantIssued, wantQueries := 1, 2
			if mode == "expired" || mode == "http401" {
				wantIssued, wantQueries = 2, 3
			} else if mode == "token403" || mode == "invalidToken" {
				wantIssued = 2
			}
			if issued != wantIssued || queries != wantQueries {
				t.Fatalf("unexpected retry counts: issued=%d queries=%d", issued, queries)
			}
		})
	}
}

func TestAliyunConcurrentRequestsAndCancellation(t *testing.T) {
	c := clientFor(t, func(http.ResponseWriter, *http.Request) {})
	p := ramProvider(t)
	var issued atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	c.http.Transport = authTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/modelstudio/cli/generateAccessToken" {
			if issued.Add(1) == 1 {
				close(entered)
				select {
				case <-release:
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
			}
			return authResponse(200, `{"cliAccessToken":"cached-token"}`)
		}
		return authResponse(200, quotaFixture)
	})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); c.provider(context.Background(), p) }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if b := c.provider(ctx, p); b.Error == "" {
		t.Error("cancelled gate wait succeeded")
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if b := c.provider(context.Background(), p); b.Error != "" {
				t.Error(b.Error)
			}
		}()
	}
	close(release)
	wg.Wait()
	if issued.Load() != 1 {
		t.Fatal("concurrent refreshes duplicated token issuance")
	}
}

func TestAliyunAccessKeyConfiguration(t *testing.T) {
	p := ramProvider(t)
	c := testConfig()
	c.Service = p.Service
	c.Providers = []ProviderConfig{p}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ProviderConfig){
		func(p *ProviderConfig) { p.Auth = "unknown" },
		func(p *ProviderConfig) { p.Kind = "mimo-token-plan" },
		func(p *ProviderConfig) { p.URL = "http://bailian-cs.console.aliyun.com" },
		func(p *ProviderConfig) { p.URL = "https://example.com" },
		func(p *ProviderConfig) { p.Region = "ap-southeast-1" },
	} {
		bad := c
		bad.Providers = []ProviderConfig{p}
		mutate(&bad.Providers[0])
		if bad.Validate() == nil {
			t.Fatal("invalid automatic-auth configuration accepted")
		}
	}
	for _, raw := range []string{ramFixture, "private-console-token", `{bad-secret-json`, `{"access_key_id":"LTAItestkey"}`, "{\n\"access_key_id\":\"LTAItestkey\",\"access_key_secret\":\"test-ram-secret\"\n}"} {
		if err := os.WriteFile(p.KeyFile, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		err := p.ValidateCredentials()
		if (err == nil) != (raw == ramFixture) {
			t.Fatal("unexpected credentials validation result")
		}
		if err != nil && strings.Contains(err.Error(), "test-ram-secret") {
			t.Fatal("credentials in validation error")
		}
	}
}

func TestAliyunFreshExpiredTokenAndCancelledExchange(t *testing.T) {
	for _, mode := range []string{"expired", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			c := clientFor(t, func(http.ResponseWriter, *http.Request) {})
			p := ramProvider(t)
			issued, queries := 0, 0
			c.http.Transport = authTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/modelstudio/cli/generateAccessToken" {
					issued++
					if mode == "cancel" {
						<-r.Context().Done()
						return nil, r.Context().Err()
					}
					return authResponse(200, `{"cliAccessToken":"fresh-token"}`)
				}
				queries++
				return authResponse(200, `{"data":{"success":false,"errorCode":"BailianGateway.Login.NotLogined"}}`)
			})
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			if b := c.provider(ctx, p); b.Error == "" {
				t.Fatal("expected rejected token or cancellation")
			}
			wantQueries := 1
			if mode == "cancel" {
				wantQueries = 0
			}
			if issued != 1 || queries != wantQueries || len(c.aliyunTokens) != 0 {
				t.Fatal("retried fresh invalid token or cached a cancelled exchange")
			}
		})
	}
}

// Explicit opt-in; queries only the Aliyun quota, with no NewAPI writes or inference calls.
func TestLiveAliyunAccessKeyReadOnly(t *testing.T) {
	path := os.Getenv("NEWAPI_ALIYUN_KEY_FILE")
	if path == "" {
		t.Skip("set NEWAPI_ALIYUN_KEY_FILE to an existing RAM credentials file")
	}
	c := clientFor(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected gateway request") })
	p := ProviderConfig{Kind: "aliyun-token-plan", Auth: "access-key", Service: config.Service{URL: "https://bailian-cs.console.aliyun.com", KeyFile: path}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for i := 0; i < 2; i++ {
		b := c.provider(ctx, p)
		if b.Error != "" || b.Value == "" {
			t.Fatal(b.Error)
		}
		t.Logf("read-only query %d: monthly remaining %s", i+1, b.Value)
	}
	if len(c.aliyunTokens) != 1 {
		t.Fatal("token not cached")
	}
}
