package netease

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSMSFlowStoresSessionOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	calls := []string{}
	c := testClient(t, path, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/weapi/sms/captcha/sent":
			fmt.Fprint(w, `{"code":200,"data":true}`)
		case "/weapi/w/login/cellphone":
			http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "session-token", Path: "/", Secure: true})
			fmt.Fprint(w, `{"code":200,"profile":{"userId":42,"nickname":"用户"}}`)
		default:
			t.Error("unexpected SMS endpoint", r.URL.Path)
		}
	})
	if err := c.SendSMS(context.Background(), "13800138000"); err != nil {
		t.Fatal(err)
	}
	p, err := c.LoginSMS(context.Background(), "13800138000", "123456")
	if err != nil || p.ID != 42 {
		t.Fatal(p, err)
	}
	if len(calls) != 2 {
		t.Fatal(calls)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "13800138000") || strings.Contains(string(data), "123456") {
		t.Fatal("phone/code persisted")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("session not private")
	}
}
func TestSMSValidationAndErrors(t *testing.T) {
	calls := 0
	response := `{"code":502,"message":"secret 13800138000 123456"}`
	c := testClient(t, "", func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, response) })
	if c.SendSMS(context.Background(), "123") == nil {
		t.Fatal("invalid phone accepted")
	}
	if _, err := c.LoginSMS(context.Background(), "13800138000", "bad"); err == nil {
		t.Fatal("invalid code accepted")
	}
	if calls != 0 {
		t.Fatal("invalid input reached API")
	}
	if _, err := c.LoginSMS(context.Background(), "13800138000", "123456"); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
	response = `{"code":8821}`
	if err := c.SendSMS(context.Background(), "13800138000"); !errors.Is(err, ErrLoginRestricted) {
		t.Fatal(err)
	}
	response = `{"code":200,"profile":{"userId":42}}`
	if _, err := c.LoginSMS(context.Background(), "13800138000", "123456"); err == nil {
		t.Fatal("login without cookie accepted")
	}
}
