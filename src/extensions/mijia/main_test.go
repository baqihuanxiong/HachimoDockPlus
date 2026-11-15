package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCloudProtocolAndDeviceControl(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var writes atomic.Int32
	var mismatch atomic.Bool
	var rejectWrite atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/miot-spec-v2/instance" {
			io.WriteString(w, `{"services":[{"iid":2,"description":"Light","properties":[{"iid":1,"description":"On","type":"urn:miot-spec-v2:property:on:0","format":"bool","access":["read","write"]},{"iid":2,"description":"Brightness","format":"uint8","access":["read","write"],"value-range":[0,100,1]}],"actions":[{"iid":1,"description":"Toggle","in":[]}]}]}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearertest-access" {
			t.Error("cloud token not supplied")
		}
		wrapped, _ := base64.StdEncoding.DecodeString(r.Header.Get("X-Client-Secret"))
		key, err := rsa.DecryptPKCS1v15(rand.Reader, private, wrapped)
		if err != nil {
			t.Error(err)
			return
		}
		body, _ := io.ReadAll(r.Body)
		encrypted, _ := base64.StdEncoding.DecodeString(string(body))
		plain, err := decrypt(encrypted, key)
		if err != nil {
			t.Error(err)
			return
		}
		var input map[string]any
		if json.Unmarshal(plain, &input) != nil {
			t.Error("invalid request JSON")
		}
		var result any
		switch r.URL.Path {
		case "/app/v2/homeroom/gethome":
			result = json.RawMessage(`{"homelist":[{"id":"12345678901234567","uid":"owner-private","name":"Home","dids":[],"roomlist":[{"id":"98765432101234567","name":"Living","dids":[]}]}],"has_more":true,"max_id":"next-page"}`)
		case "/app/v2/homeroom/get_dev_room_page":
			if input["start_id"] != "next-page" {
				t.Error("pagination cursor missing")
			}
			result = json.RawMessage(`{"info":[{"id":12345678901234567,"dids":["lamp"],"roomlist":[{"id":98765432101234567,"dids":["lamp"]}]}],"has_more":false}`)
		case "/app/v2/home/device_list_page":
			result = map[string]any{"list": []any{map[string]any{"did": "lamp", "name": "Lamp", "model": "test.light", "spec_type": "urn:miot-spec-v2:device:light:test", "isOnline": true, "token": "must-not-leak", "localip": "private-ip"}}}
		case "/app/appgateway/miot/appsceneservice/AppSceneService/GetManualSceneList":
			result = []any{map[string]any{"scene_id": "scene", "scene_name": "Home scene", "room_id": 2}}
		case "/app/v2/miotspec/prop/set":
			writes.Add(1)
			code := 0
			if rejectWrite.Load() {
				code = -704220043
			}
			result = []any{map[string]any{"did": "lamp", "siid": 2, "piid": 1, "code": code}}
		case "/app/v2/miotspec/prop/get":
			result = []any{map[string]any{"did": "lamp", "siid": 2, "piid": 1, "code": 0, "value": !mismatch.Load()}}
		case "/app/v2/miotspec/action":
			writes.Add(1)
			result = map[string]any{"code": 0}
		case "/app/appgateway/miot/appsceneservice/AppSceneService/NewRunScene":
			writes.Add(1)
			result = true
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		response, _ := json.Marshal(map[string]any{"code": 0, "result": result})
		response, _ = encrypt(response, key)
		io.WriteString(w, base64.StdEncoding.EncodeToString(response))
	}))
	defer server.Close()
	a, closeFn, err := openApp(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	a.auth = credentials{AccessToken: "test-access", RefreshToken: "test-refresh", ExpiresAt: time.Now().Unix() + 3600}
	a.cloud.cloudURL = server.URL
	a.cloud.specURL = server.URL
	a.cloud.key = &private.PublicKey
	ctx := context.Background()
	if _, err = a.run(ctx, []string{"devices", "refresh"}); err != nil {
		t.Fatal(err)
	}
	if len(a.cache.Devices) != 1 || a.cache.Devices[0].Home != "Home" || a.cache.Devices[0].Room != "Living" {
		t.Fatal("mixed string/numeric pagination IDs lost the device or its room")
	}
	a.cache.Scenes[0].RoomID = json.Number("98765432101234567")
	if err = a.write("devices.json", a.cache); err != nil {
		t.Fatal(err)
	}
	var reloaded cache
	if err = a.read("devices.json", &reloaded); err != nil || scalar(reloaded.Scenes[0].RoomID) != "98765432101234567" {
		t.Fatal("numeric ID changed in persisted cache", err)
	}
	for _, args := range [][]string{{"devices", "list"}, {"device", "spec", "lamp"}, {"device", "get", "lamp"}, {"scenes", "list"}} {
		result, err := a.run(ctx, args)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(result)
		for _, secret := range []string{"must-not-leak", "private-ip", "owner-private", "test-access"} {
			if strings.Contains(string(b), secret) {
				t.Fatal("private metadata exposed")
			}
		}
	}
	for _, args := range [][]string{{"device", "set", "unknown", "2", "1", "true"}, {"device", "set", "lamp", "2", "1", `"true"`}, {"device", "set", "lamp", "2", "2", "200"}, {"device", "action", "lamp", "2", "1", "[1]"}} {
		if _, err = a.run(ctx, args); err == nil {
			t.Fatal("invalid control accepted", args)
		}
	}
	if writes.Load() != 0 {
		t.Fatal("validation sent device commands")
	}
	result, err := a.run(ctx, []string{"device", "set", "lamp", "2", "1", "true"})
	if err != nil || result.(map[string]string)["status"] != "verified" {
		t.Fatal(result, err)
	}
	mismatch.Store(true)
	result, err = a.run(ctx, []string{"device", "set", "lamp", "2", "1", "true"})
	if err != nil || result.(map[string]string)["status"] != "accepted" {
		t.Fatal(result, err)
	}
	if writes.Load() != 2 {
		t.Fatal("write was retried")
	}
	rejectWrite.Store(true)
	if _, err = a.run(ctx, []string{"device", "set", "lamp", "2", "1", "true"}); err == nil || !strings.Contains(err.Error(), "-704220043") {
		t.Fatal("failed write treated as accepted", err)
	}
	if writes.Load() != 3 {
		t.Fatal("failed write was retried")
	}
	if _, err = a.run(ctx, []string{"device", "action", "lamp", "2", "1", "[]"}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.run(ctx, []string{"scenes", "run", "scene"}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationStateRefreshAndLogout(t *testing.T) {
	a, closeFn, err := openApp(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	var tokenCalls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		io.WriteString(w, `{"code":0,"result":{"access_token":"private-access","refresh_token":"private-refresh","expires_in":3600}}`)
	}))
	defer s.Close()
	a.cloud.cloudURL = s.URL
	if _, err = a.run(context.Background(), []string{"auth", "login"}); err != nil {
		t.Fatal(err)
	}
	var pending pendingLogin
	a.read("pending-login.json", &pending)
	file := filepath.Join(t.TempDir(), "result")
	writeCode := func(state string) {
		raw, _ := json.Marshal(map[string]string{"state": state, "code": "single-use-code"})
		os.WriteFile(file, []byte(base64.StdEncoding.EncodeToString(raw)), 0600)
	}
	writeCode("wrong-state")
	if _, err = a.run(context.Background(), []string{"auth", "authorize", "--file", file}); err == nil {
		t.Fatal("invalid OAuth state accepted")
	}
	if tokenCalls.Load() != 0 {
		t.Fatal("invalid state reached cloud")
	}
	writeCode(pending.State)
	out, err := a.run(context.Background(), []string{"auth", "authorize", "--file", file})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "private-") {
		t.Fatal("token exposed")
	}
	a.auth.ExpiresAt = 0
	if _, err = a.token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tokenCalls.Load() != 2 {
		t.Fatal("refresh not performed")
	}
	st, _ := os.Stat(filepath.Join(a.root, "credentials.json"))
	if st.Mode().Perm() != 0600 {
		t.Fatal("credentials permissions")
	}
	if _, err = a.run(context.Background(), []string{"auth", "logout"}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.token(context.Background()); err == nil {
		t.Fatal("logged-out account used")
	}
}

func TestCryptoRejectsBadPaddingAndCloudFailures(t *testing.T) {
	block, _ := pem.Decode([]byte(publicKey))
	if block == nil {
		t.Fatal("public key missing")
	}
	if _, err := x509.ParsePKIXPublicKey(block.Bytes); err != nil {
		t.Fatal(err)
	}
	key := []byte("0123456789abcdef")
	for _, text := range []string{"", "test", strings.Repeat("x", 16)} {
		encrypted, err := encrypt([]byte(text), key)
		if err != nil {
			t.Fatal(err)
		}
		plain, err := decrypt(encrypted, key)
		if err != nil || string(plain) != text {
			t.Fatal("AES roundtrip")
		}
	}
	if _, err := decrypt([]byte("invalid"), key); err == nil {
		t.Fatal("invalid cipher accepted")
	}
	for _, raw := range []string{`[]`, `{}`, `[{"code":-1}]`} {
		if checkResult(json.RawMessage(raw)) == nil {
			t.Fatal("failure treated as success", raw)
		}
	}
}
