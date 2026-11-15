// Cloud protocol adapted from HachimoDock's miot_protocol.rs and Xiaomi
// miloco-miot (cloud.py/const.py). Copyright (C) 2025 Xiaomi Corporation.
// See Xiaomi-Miloco-LICENSE.md and THIRD_PARTY_NOTICES.md in this directory.
package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const clientID = "2882303761520431603"
const redirectURI = "https://mico.api.mijia.tech/login_redirect"
const cloudHost = "https://mico.api.mijia.tech"
const specHost = "https://miot-spec.org"
const responseLimit = 4 << 20

const publicKey = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAzH220YGgZOlXJ4eSleFb
Beylq4qHsVNzhPTUTy/caDb4a3GzqH6SX4GiYRilZZZrjjU2ckkr8GM66muaIuJw
r8ZB9SSY3Hqwo32tPowpyxobTN1brmqGK146X6JcFWK/QiUYVXZlcHZuMgXLlWyn
zTMVl2fq7wPbzZwOYFxnSRh8YEnXz6edHAqJqLEqZMP00bNFBGP+yc9xmc7ySSyw
OgW/muVzfD09P2iWhl3x8N+fBBWpuI5HjvyQuiX8CZg3xpEeCV8weaprxMxR0epM
3l7T6rJuPXR1D7yhHaEQj2+dyrZTeJO8D8SnOgzV5j4bp1dTunlzBXGYVjqDsRhZ
qQIDAQAB
-----END PUBLIC KEY-----`

type cloudClient struct {
	client            *http.Client
	cloudURL, specURL string
	key               *rsa.PublicKey
}

func newCloudClient() *cloudClient {
	return &cloudClient{client: &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("API redirect refused") }}, cloudURL: cloudHost, specURL: specHost}
}

func (c *cloudClient) request(ctx context.Context, method, endpoint string, headers map[string]string, data []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.New("网络请求未取得结果；控制请求不会自动重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("服务返回 HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit+1))
	if err != nil {
		return nil, errors.New("服务响应中断")
	}
	if len(body) > responseLimit {
		return nil, errors.New("服务响应超出上限")
	}
	return body, nil
}

func (c *cloudClient) token(ctx context.Context, data any) (credentials, error) {
	var auth credentials
	b, _ := json.Marshal(data)
	raw, err := c.request(ctx, "GET", c.cloudURL+"/app/v2/mico/oauth/get_token?"+url.Values{"data": {string(b)}}.Encode(), nil, nil)
	if err != nil {
		return auth, err
	}
	var envelope struct {
		Code   int         `json:"code"`
		Result credentials `json:"result"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.Code != 0 || envelope.Result.AccessToken == "" || envelope.Result.RefreshToken == "" || envelope.Result.ExpiresIn <= 0 {
		return auth, errors.New("账号授权未完成或已失效")
	}
	auth = envelope.Result
	auth.ExpiresAt = time.Now().Unix() + auth.ExpiresIn*7/10
	return auth, nil
}

func encrypt(data, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padding := aes.BlockSize - len(data)%aes.BlockSize
	plain := append(append([]byte(nil), data...), bytes.Repeat([]byte{byte(padding)}, padding)...)
	out := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, key).CryptBlocks(out, plain)
	return out, nil
}

func decrypt(data, key []byte) ([]byte, error) {
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return nil, errors.New("响应密文无效")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, key).CryptBlocks(out, data)
	padding := int(out[len(out)-1])
	if padding < 1 || padding > aes.BlockSize || !bytes.Equal(out[len(out)-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
		return nil, errors.New("响应填充无效")
	}
	return out[:len(out)-padding], nil
}

func (c *cloudClient) post(ctx context.Context, token, path string, data any) (json.RawMessage, error) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	rsaKey := c.key
	if rsaKey == nil {
		block, _ := pem.Decode([]byte(publicKey))
		if block == nil {
			return nil, errors.New("内置公钥无效")
		}
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey = parsed.(*rsa.PublicKey)
	}
	secret, err := rsa.EncryptPKCS1v15(rand.Reader, rsaKey, key)
	if err != nil {
		return nil, err
	}
	plain, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	encrypted, err := encrypt(plain, key)
	if err != nil {
		return nil, err
	}
	raw, err := c.request(ctx, "POST", c.cloudURL+path, map[string]string{
		"Content-Type": "text/plain", "User-Agent": "mico/docker", "X-Client-BizId": "micoapi", "X-Encrypt-Type": "1", "X-Client-AppId": clientID, "X-Client-Secret": base64.StdEncoding.EncodeToString(secret), "Authorization": "Bearer" + token,
	}, []byte(base64.StdEncoding.EncodeToString(encrypted)))
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		return nil, errors.New("服务响应编码无效")
	}
	decoded, err := decrypt(ciphertext, key)
	if err != nil {
		return nil, err
	}
	var response struct {
		Code   *int            `json:"code"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal(decoded, &response) != nil || response.Code == nil {
		return nil, errors.New("服务响应格式无效")
	}
	if *response.Code != 0 {
		return nil, fmt.Errorf("云端拒绝请求，代码 %d", *response.Code)
	}
	if len(response.Result) == 0 {
		return nil, errors.New("服务缺少执行结果")
	}
	return response.Result, nil
}
