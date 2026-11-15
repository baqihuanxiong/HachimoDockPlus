package newapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hachimodockplus/internal/config"
)

func ok(w http.ResponseWriter, v any) {
	json.NewEncoder(w).Encode(map[string]any{"success": true, "data": v})
}
func clientFor(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	c := testConfig()
	key := filepath.Join(t.TempDir(), "admin.key")
	if err := os.WriteFile(key, []byte("test-admin-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	c.Service = config.Service{URL: server.URL, KeyFile: key}
	client, err := NewClient(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestUsagePaginationWindowAggregationAndPartial(t *testing.T) {
	var logPages atomic.Int32
	loc, _ := time.LoadLocation("Asia/Shanghai")
	sampled := time.Date(2026, 11, 15, 12, 0, 0, 0, loc)
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-admin-secret" || r.Header.Get("New-Api-User") != "1" {
			t.Error("missing administrator headers")
		}
		switch r.URL.Path {
		case "/api/status":
			ok(w, map[string]any{"quota_per_unit": 500000})
		case "/api/log/":
			logPages.Add(1)
			start, _ := strconv.ParseInt(r.URL.Query().Get("start_timestamp"), 10, 64)
			end, _ := strconv.ParseInt(r.URL.Query().Get("end_timestamp"), 10, 64)
			loc, _ := time.LoadLocation("Asia/Shanghai")
			if time.Unix(start, 0).In(loc).Hour() != 0 || end-start < 6*86400 {
				t.Error("wrong calendar window")
			}
			p, _ := strconv.Atoi(r.URL.Query().Get("p"))
			var items []map[string]any
			for i := (p - 1) * 100; i < min(102, p*100); i++ {
				kind := 2
				quota := 500000
				uid := 1
				if i == 101 {
					kind = 5
				}
				if i == 100 {
					quota = 1000000
					uid = 2
				}
				items = append(items, map[string]any{"id": i + 1, "type": kind, "quota": quota, "user_id": uid, "username": fmt.Sprintf("user%d", uid), "created_at": end - 3600, "model_name": "qwen", "channel": 1})
			}
			ok(w, map[string]any{"total": 102, "items": items})
		case "/api/channel/":
			ok(w, map[string]any{"total": 3, "items": []any{map[string]any{"id": 1, "name": "Qwen", "status": 1, "response_time": 120}, map[string]any{"id": 2, "name": "disabled", "status": 2}, map[string]any{"id": 3, "name": "no samples", "status": 1}}})
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	c.cfg.TopN = 1

	d, err := c.fetchAt(context.Background(), sampled)
	if err != nil {
		t.Fatal(err)
	}
	if d.Partial || logPages.Load() != 2 || d.Month.USD != 102 || d.TotalUSD != 102 || d.Success != 101 || d.Failed != 1 || len(d.Models) != 1 || d.Models[0].Calls != 102 || len(d.Users) != 1 || d.Users[0].ID != 1 || d.Users[0].USD != 100 {
		t.Fatalf("bad aggregation: %+v", d)
	}
	c.cfg.MaxLogs = 100
	d, err = c.fetchAt(context.Background(), sampled)
	if err != nil || !d.Partial || d.Success != 100 {
		t.Fatal(d, err)
	}
}

func TestCalendarMetricsAndRollingRankingsAcrossMonthAndWeek(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	sampled := time.Date(2026, 11, 2, 12, 0, 0, 0, loc)
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status":
			ok(w, map[string]any{"quota_per_unit": 500000})
		case "/api/log/":
			start, _ := strconv.ParseInt(r.URL.Query().Get("start_timestamp"), 10, 64)
			if time.Unix(start, 0).In(loc).Format("2006-01-02") != "2026-10-27" {
				t.Error("scan omitted prior-month ranking window")
			}
			var rows []map[string]any
			for i, row := range []struct {
				date  string
				quota int
				kind  int
				model string
			}{{"2026-11-02 11:00", 1, 2, "A"}, {"2026-11-02 11:01", 0, 5, "A"}, {"2026-11-01 12:00", 2, 2, "B"}, {"2026-10-31 12:00", 4, 2, "B"}, {"2026-10-27 12:00", 8, 2, "B"}, {"2026-10-26 12:00", 16, 2, "B"}, {"2026-11-02 13:00", 32, 2, "B"}} {
				at, _ := time.ParseInLocation("2006-01-02 15:04", row.date, loc)
				rows = append(rows, map[string]any{"id": i + 1, "type": row.kind, "created_at": at.Unix(), "quota": row.quota * 500000, "user_id": 1, "model_name": row.model})
			}
			ok(w, map[string]any{"total": len(rows), "items": rows})
		default:
			t.Errorf("single page must not request channels: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	d, err := c.fetchAt(context.Background(), sampled)
	if err != nil {
		t.Fatal(err)
	}
	if d.Today.USD != 1 || d.Today.Calls() != 2 || d.Today.Success != 1 || d.Week != d.Today || d.Month.USD != 3 || d.Month.Calls() != 3 || d.TotalUSD != 15 || d.Success != 4 || d.Failed != 1 || d.LogCount != 5 {
		t.Fatalf("wrong periods: %+v", d)
	}
	if len(d.Models) != 2 || d.Models[0].Name != "B" || d.Models[0].Calls != 3 || d.Users[0].USD != 15 {
		t.Fatal(d)
	}
}

func TestResetAllPagesOverrideAndVerification(t *testing.T) {
	var writes atomic.Int32
	balances := map[int]int64{}
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status":
			ok(w, map[string]any{"quota_per_unit": 500000})
		case "/api/user/self":
			ok(w, map[string]any{"id": 1, "role": 100})
		case "/api/user/":
			p, _ := strconv.Atoi(r.URL.Query().Get("p"))
			var users []any
			for i := (p-1)*100 + 1; i <= min(101, p*100); i++ {
				users = append(users, map[string]any{"id": i, "role": 1})
			}
			ok(w, map[string]any{"total": 101, "items": users})
		case "/api/user/manage":
			if r.Method != "POST" {
				t.Error("reset must use manage endpoint")
			}
			var payload struct {
				ID           int `json:"id"`
				Action, Mode string
				Value        int64
			}
			json.NewDecoder(r.Body).Decode(&payload)
			if payload.Action != "add_quota" || payload.Mode != "override" || payload.Value != 100000000 {
				t.Errorf("unsafe reset payload: %+v", payload)
			}
			balances[payload.ID] = payload.Value
			writes.Add(1)
			ok(w, nil)
		default:
			id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/user/"))
			ok(w, map[string]any{"id": id, "quota": balances[id]})
		}
	})
	r := c.Reset(context.Background(), 200, func(ResetResult) {})
	if r.Error != "" || r.Total != 101 || r.Done != 101 || writes.Load() != 101 {
		t.Fatal(r, writes.Load())
	}
}

func TestResetStopsOnPermissionPartialFailureAndFalseSuccess(t *testing.T) {
	for _, mode := range []string{"permission", "failed-write", "false-success", "missing-total", "duplicate-page"} {
		t.Run(mode, func(t *testing.T) {
			var writes atomic.Int32
			c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/status":
					ok(w, map[string]any{"quota_per_unit": 500000})
				case "/api/user/self":
					role := 100
					if mode == "permission" {
						role = 10
					}
					ok(w, map[string]any{"id": 1, "role": role})
				case "/api/user/":
					d := map[string]any{"total": 2, "items": []any{map[string]any{"id": 1, "role": 10}, map[string]any{"id": 2, "role": 1}}}
					if mode == "missing-total" {
						delete(d, "total")
					}
					if mode == "duplicate-page" {
						d["items"] = []any{map[string]any{"id": 1}}
					}
					ok(w, d)
				case "/api/user/manage":
					n := writes.Add(1)
					if mode == "failed-write" && n == 2 {
						json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "secret body"})
					} else {
						ok(w, nil)
					}
				default:
					id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/user/"))
					quota := 100000000
					if mode == "false-success" {
						quota = 7
					}
					ok(w, map[string]any{"id": id, "quota": quota})
				}
			})
			r := c.Reset(context.Background(), 200, func(ResetResult) {})
			if r.Error == "" {
				t.Fatal("invalid reset reported success", r)
			}
			if (mode == "permission" || mode == "missing-total" || mode == "duplicate-page") && writes.Load() != 0 {
				t.Fatal("preflight failure wrote balances")
			}
			if mode == "false-success" && writes.Load() != 1 {
				t.Fatal("continued after verification failure")
			}
			if mode == "failed-write" && r.Done != 1 {
				t.Fatal("lost partial result", r)
			}
		})
	}
}

func TestRequestTimeoutCancelAndCredentialRedaction(t *testing.T) {
	for _, mode := range []string{"auth", "json", "timeout", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "auth":
					w.WriteHeader(401)
					fmt.Fprint(w, "test-admin-secret")
				case "json":
					fmt.Fprint(w, "test-admin-secret")
				default:
					<-r.Context().Done()
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			if mode == "timeout" {
				c.http.Timeout = 10 * time.Millisecond
			}
			if mode == "cancel" {
				cancel()
			}
			_, err := c.Fetch(ctx)
			if err == nil || strings.Contains(err.Error(), "test-admin-secret") || strings.Contains(err.Error(), c.cfg.URL) {
				t.Fatal("unsafe error", err)
			}
		})
	}
}

func TestProviderConsoleQueriesAndSessionFailures(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cli/api.json":
			r.ParseForm()
			if r.Header.Get("Authorization") != "Bearer test-admin-secret" || r.URL.Query().Get("api") != "zeldaHttp.apikeyMgr./tokenplan/personal/api/v2/usage" || !strings.Contains(r.Form.Get("params"), "cornerstoneParam") {
				t.Error("bad Bailian console request")
			}
			fmt.Fprint(w, `{"data":{"DataV2":{"data":{"data":{"per1MonthPercentage":0.25,"per1MonthResetTime":1795000000000}}}}}`)
		case "/v1/platform/token_plan":
			if r.Header.Get("api-key") != "test-admin-secret" || r.Header.Get("Cookie") != "" {
				t.Error("MiMo must use API key without Cookie")
			}
			fmt.Fprint(w, `{"object":"token_plan","plan_code":"lite:year","quota":{"total_credits":49200000000,"used_credits":6231352632,"remaining_credits":42968647368},"period":{"end_at":"2027-07-10 23:59:59"}}`)
		}
	})
	for _, kind := range []string{"aliyun-token-plan", "mimo-token-plan"} {
		service := c.cfg.Service
		if kind == "mimo-token-plan" {
			service.URL += "/v1/platform/token_plan"
		}
		b := c.provider(context.Background(), ProviderConfig{Kind: kind, Name: kind, Service: service})
		if b.Error != "" {
			t.Fatal(b)
		}
		if kind == "aliyun-token-plan" && !strings.Contains(b.Detail, "75.0%") || kind == "mimo-token-plan" && (!strings.Contains(b.Detail, "42968647368 / 49200000000 Credits") || !strings.Contains(b.Detail, "2027-07-10")) {
			t.Fatal(b)
		}
	}
	bad := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"success":false,"errorMsg":"test-admin-secret"},"code":401}`)
	})
	for _, kind := range []string{"aliyun-token-plan", "mimo-token-plan"} {
		b := bad.provider(context.Background(), ProviderConfig{Kind: kind, Service: bad.cfg.Service})
		if b.Error == "" || strings.Contains(b.Error, "test-admin-secret") {
			t.Fatal(b)
		}
	}
}

func TestLoadConfigDefaultsAndValidation(t *testing.T) {
	root, err := config.Parse([]byte(`{"data_dir":"data","apps":{"newapi-monitor":{"url":"https://gateway.example","key_file":"secrets/admin.key"}}}`), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.ResetUSD != 200 || c.TopN != 10 || !filepath.IsAbs(c.KeyFile) {
		t.Fatal(c)
	}
	for _, mutate := range []func(*Config){func(c *Config) { c.TopN = 0 }, func(c *Config) { c.URL = "https://user:secret@example.com" }, func(c *Config) { c.ChannelIDs = []int{1, 1} }, func(c *Config) { c.ResetUSD = -1 }, func(c *Config) { c.Timezone = "invalid" }, func(c *Config) { c.Providers = []ProviderConfig{{Kind: "unknown"}} }} {
		bad := c
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("bad configuration accepted")
		}
	}
	c.KeyFile = filepath.Join(t.TempDir(), "insecure.key")
	if err = os.WriteFile(c.KeyFile, []byte("key"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = NewClient(c); err == nil {
		t.Fatal("insecure key permissions accepted")
	}
}

// Opt-in read-only smoke test; never sends a reset, probe or inference request.
func TestLiveReadOnly(t *testing.T) {
	path := os.Getenv("NEWAPI_MONITOR_CONFIG")
	if path == "" {
		t.Skip("set NEWAPI_MONITOR_CONFIG to an existing local configuration")
	}
	root, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	d, err := c.Fetch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("read-only: logs=%d models=%d top_users=%d success=%d failed=%d partial=%v", d.LogCount, len(d.Models), len(d.Users), d.Success, d.Failed, d.Partial)
	for _, b := range d.Balances {
		if b.Error != "" {
			t.Errorf("configured provider: %s", b.Error)
		} else {
			t.Log("configured provider read-only query passed")
		}
	}
	if d.Month.Calls() < d.Today.Calls() || d.Week.Calls() < d.Today.Calls() {
		t.Fatal("inconsistent period counts")
	}
}
