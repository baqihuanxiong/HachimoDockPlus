package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadProperties(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	properties := []property{
		{SIID: 2, PIID: 1, Access: []string{"read"}},
		{SIID: 3, PIID: 1, Access: []string{"read"}},
		{SIID: 3, PIID: 2, Access: []string{"write"}},
	}
	for _, tt := range []struct {
		name       string
		args       []string
		response   string
		wantParams int
		wantRows   int
		wantError  string
	}{
		{"partial power reading", nil, `[{"did":"cabinet","siid":2,"piid":1,"code":-704220043,"value":999},{"did":"cabinet","siid":3,"piid":1,"code":0,"value":42.5}]`, 2, 2, ""},
		{"targeted power", []string{"3", "1"}, `[{"did":"cabinet","siid":3,"piid":1,"code":0,"value":42.5}]`, 1, 1, ""},
		{"zero power", []string{"3", "1"}, `[{"did":"cabinet","siid":3,"piid":1,"code":0,"value":0}]`, 1, 1, ""},
		{"false value", []string{"2", "1"}, `[{"did":"cabinet","siid":2,"piid":1,"code":0,"value":false}]`, 1, 1, ""},
		{"all failed", nil, `[{"did":"cabinet","siid":2,"piid":1,"code":-704220043},{"did":"cabinet","siid":3,"piid":1,"code":-704040003}]`, 2, 0, "SIID=2 PIID=1：属性值无效，云端未提供有效读数（代码 -704220043）"},
		{"target failed", []string{"3", "1"}, `[{"did":"cabinet","siid":3,"piid":1,"code":-704220043}]`, 1, 0, "SIID=3 PIID=1"},
		{"empty result", nil, `[]`, 2, 0, "设备未返回属性结果"},
		{"missing code", nil, `[{"did":"cabinet","siid":3,"piid":1,"value":42.5}]`, 2, 0, "属性结果缺少状态码"},
		{"missing value", nil, `[{"did":"cabinet","siid":3,"piid":1,"code":0}]`, 2, 0, "属性结果缺少有效读数"},
		{"invalid result", nil, `{}`, 2, 0, "属性响应无效"},
		{"write only", []string{"3", "2"}, "", 0, 0, "设备属性不存在或不可读"},
		{"unknown property", []string{"99", "1"}, "", 0, 0, "设备属性不存在或不可读"},
		{"invalid id", []string{"x", "1"}, "", 0, 0, "能力 ID 必须为正整数"},
		{"zero id", []string{"0", "1"}, "", 0, 0, "能力 ID 必须为正整数"},
		{"missing id", []string{"3"}, "", 0, 0, "用法：device get DID [SIID PIID]"},
		{"all batches retained", nil, "batches", 100, 101, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/app/v2/miotspec/prop/get" || tt.wantParams == 0 {
					t.Errorf("unexpected cloud call: %s", r.URL.Path)
					return
				}
				wrapped, _ := base64.StdEncoding.DecodeString(r.Header.Get("X-Client-Secret"))
				key, err := rsa.DecryptPKCS1v15(rand.Reader, private, wrapped)
				if err != nil {
					t.Error(err)
					return
				}
				body, _ := io.ReadAll(r.Body)
				ciphertext, _ := base64.StdEncoding.DecodeString(string(body))
				plain, err := decrypt(ciphertext, key)
				var input struct {
					Datasource int              `json:"datasource"`
					Params     []propertyResult `json:"params"`
				}
				if err != nil || json.Unmarshal(plain, &input) != nil {
					t.Error("invalid encrypted request")
					return
				}
				wantParams := tt.wantParams
				if tt.response == "batches" && calls == 2 {
					wantParams = 1
				}
				if input.Datasource != 1 || len(input.Params) != wantParams {
					t.Errorf("unexpected read request: %s", plain)
				}
				for _, p := range input.Params {
					if p.DID != "cabinet" || len(tt.args) == 2 && fmt.Sprint(p.SIID, "/", p.PIID) != strings.Join(tt.args, "/") {
						t.Errorf("unexpected property: %+v", p)
					}
				}
				result := json.RawMessage(tt.response)
				if tt.response == "batches" {
					for i := range input.Params {
						code := -704220043
						if calls == 2 {
							code = 0
							input.Params[i].Value = 42.5
						}
						input.Params[i].Code = &code
					}
					result, _ = json.Marshal(input.Params)
				}
				response, _ := json.Marshal(map[string]any{"code": 0, "result": result})
				response, _ = encrypt(response, key)
				io.WriteString(w, base64.StdEncoding.EncodeToString(response))
			}))
			defer s.Close()
			a, closeFn, err := openApp(context.Background(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer closeFn()
			a.auth = credentials{AccessToken: "test-access", ExpiresAt: time.Now().Unix() + 3600}
			a.cloud.cloudURL, a.cloud.key = s.URL, &private.PublicKey
			a.cache.Devices = []device{{DID: "cabinet", Name: "弱电柜"}}
			a.cache.Specs["cabinet"] = spec{Properties: properties}
			if tt.response == "batches" {
				props := []property{}
				for i := 1; i <= 101; i++ {
					props = append(props, property{SIID: 2, PIID: i, Access: []string{"read"}})
				}
				a.cache.Specs["cabinet"] = spec{Properties: props}
			}
			out, err := a.run(context.Background(), append([]string{"device", "get", "cabinet"}, tt.args...))
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("got %v, want error containing %q", err, tt.wantError)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				rows := out.([]propertyResult)
				if len(rows) != tt.wantRows {
					t.Fatalf("got %d rows, want %d", len(rows), tt.wantRows)
				}
				for _, row := range rows {
					if *row.Code != 0 && (row.Value != nil || !strings.Contains(row.Error, "-704220043")) {
						t.Fatalf("failed property not identified: %+v", row)
					}
				}
				var wantValue any = float64(42.5)
				if tt.name == "zero power" {
					wantValue = float64(0)
				} else if tt.name == "false value" {
					wantValue = false
				}
				if rows[len(rows)-1].Value != wantValue {
					t.Fatalf("got reading %v, want %v", rows[len(rows)-1].Value, wantValue)
				}
				encoded, _ := json.Marshal(rows)
				for _, value := range []any{float64(42.5), float64(0), false} {
					if rows[len(rows)-1].Value == value && !strings.Contains(string(encoded), fmt.Sprintf(`"value":%v`, value)) {
						t.Fatalf("valid reading omitted: %s", encoded)
					}
				}
			}
			wantCalls := 1
			if tt.wantParams == 0 {
				wantCalls = 0
			} else if tt.response == "batches" {
				wantCalls = 2
			}
			if calls != wantCalls {
				t.Fatalf("got %d cloud calls, want %d (no retries)", calls, wantCalls)
			}
		})
	}
}
