package newapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
)

var errAliyunSessionExpired = errors.New("阿里控制台会话失效，请更新凭证或配置 RAM 自动续期")

type aliyunAccessKey struct {
	ID            string `json:"access_key_id"`
	Secret        string `json:"access_key_secret"`
	SecurityToken string `json:"security_token,omitempty"`
}

type aliyunToken struct {
	credentialHash [32]byte
	value          string
}

func parseAliyunAccessKey(raw string) (aliyunAccessKey, error) {
	var key aliyunAccessKey
	if len(raw) > 64<<10 || json.Unmarshal([]byte(raw), &key) != nil || !(strings.HasPrefix(key.ID, "LTAI") || strings.HasPrefix(key.ID, "STS.")) || key.Secret == "" || strings.ContainsAny(key.ID+key.Secret+key.SecurityToken, " \t\r\n") {
		return aliyunAccessKey{}, errors.New("阿里 RAM 凭证须为单行 JSON，包含 access_key_id 和 access_key_secret")
	}
	return key, nil
}

func aliyunRegion(p ProviderConfig) string {
	if p.Region == "" {
		return "cn-beijing"
	}
	return p.Region
}

func validAliyunGateway(p ProviderConfig) bool {
	u := strings.TrimRight(p.URL, "/")
	if aliyunRegion(p) == "cn-beijing" {
		return u == "https://bailian-cs.console.aliyun.com" || u == "https://bailian-cs.console.alibabacloud.com"
	}
	return u == "https://modelstudio-cs.console.aliyun.com" || u == "https://bailian-singapore-cs.alibabacloud.com"
}

func (c *Client) aliyunWithAccessKey(ctx context.Context, p ProviderConfig, raw string) (Balance, error) {
	key, err := parseAliyunAccessKey(raw)
	if err != nil {
		return Balance{}, err
	}
	select {
	case c.aliyunGate <- struct{}{}:
		defer func() { <-c.aliyunGate }()
	case <-ctx.Done():
		return Balance{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return Balance{}, err
	}
	// Replacing a credentials file invalidates the old token; keep one entry per provider.
	id := p.URL + "\x00" + aliyunRegion(p) + "\x00" + p.KeyFile
	fingerprint := sha256.Sum256([]byte(raw))
	cached, exists := c.aliyunTokens[id]
	if exists && cached.credentialHash != fingerprint {
		delete(c.aliyunTokens, id)
		exists = false
	}
	minted := false
	if !exists {
		cached.value, err = c.generateAliyunToken(ctx, p, key)
		if err != nil {
			return Balance{}, err
		}
		cached.credentialHash = fingerprint
		c.aliyunTokens[id] = cached
		minted = true
	}
	balance, err := c.aliyun(ctx, p, cached.value)
	if errors.Is(err, errAliyunSessionExpired) {
		delete(c.aliyunTokens, id)
		// Mint at most once per refresh, even when the replacement token is rejected.
		if !minted {
			cached.value, err = c.generateAliyunToken(ctx, p, key)
			if err != nil {
				return Balance{}, err
			}
			balance, err = c.aliyun(ctx, p, cached.value)
			if !errors.Is(err, errAliyunSessionExpired) {
				c.aliyunTokens[id] = cached
			}
		}
	}
	return balance, err
}

func (c *Client) generateAliyunToken(ctx context.Context, p ProviderConfig, key aliyunAccessKey) (string, error) {
	host := "modelstudio." + aliyunRegion(p) + ".aliyuncs.com"
	path := "/modelstudio/cli/generateAccessToken"
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", errors.New("无法生成阿里请求签名随机数")
	}
	headers := map[string]string{
		"host": host, "content-type": "application/json",
		"x-acs-action": "GenerateCLIAccessToken", "x-acs-version": "2026-02-10",
		"x-acs-content-sha256":  sha256Hex(""),
		"x-acs-date":            time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"x-acs-signature-nonce": hex.EncodeToString(nonce),
	}
	if key.SecurityToken != "" {
		headers["x-acs-security-token"] = key.SecurityToken
	}
	signAliyunRequest(path, headers, key)
	var response struct {
		Token string `json:"cliAccessToken"`
	}
	if err := c.request(ctx, http.MethodPost, "https://"+host+path, "application/json", nil, headers, &response); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errors.New("阿里令牌获取失败，请检查 RAM 密钥、GenerateCLIAccessToken 权限、网络与系统时间")
	}
	if response.Token == "" || len(response.Token) > 64<<10 || strings.ContainsAny(response.Token, " \t\r\n") {
		return "", errors.New("阿里未返回有效控制台令牌")
	}
	return response.Token, nil
}

// ACS3-HMAC-SHA256 signs an empty POST body and no query parameters.
func signAliyunRequest(path string, headers map[string]string, key aliyunAccessKey) {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	canonicalHeaders := ""
	for _, name := range names {
		canonicalHeaders += name + ":" + strings.TrimSpace(headers[name]) + "\n"
	}
	signed := strings.Join(names, ";")
	canonical := "POST\n" + path + "\n\n" + canonicalHeaders + "\n" + signed + "\n" + sha256Hex("")
	mac := hmac.New(sha256.New, []byte(key.Secret))
	_, _ = mac.Write([]byte("ACS3-HMAC-SHA256\n" + sha256Hex(canonical)))
	headers["Authorization"] = "ACS3-HMAC-SHA256 Credential=" + key.ID + ",SignedHeaders=" + signed + ",Signature=" + hex.EncodeToString(mac.Sum(nil))
}

func sha256Hex(s string) string {
	digest := sha256.Sum256([]byte(s))
	return hex.EncodeToString(digest[:])
}
