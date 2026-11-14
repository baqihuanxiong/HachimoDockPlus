package netease

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type rewrite struct{ target *url.URL }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	u := *req.URL
	u.Scheme = r.target.Scheme
	u.Host = r.target.Host
	copy.URL = &u
	return http.DefaultTransport.RoundTrip(copy)
}
func testClient(t *testing.T, path string, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse(server.URL)
	c.http.Transport = rewrite{target}
	return c
}
func TestSessionPersistenceAndLogout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	c := testClient(t, path, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/weapi/w/login/cellphone" {
			http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "secret-session", Path: "/", Domain: ".music.163.com", Secure: true})
		}
		fmt.Fprint(w, `{"code":200,"profile":{"userId":42,"nickname":"用户"}}`)
	})
	if _, err := c.LoginSMS(context.Background(), "13800138000", "123456"); err != nil {
		t.Fatal(err)
	}
	restored, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	restored.http.Transport = c.http.Transport
	profile, err := restored.Account(context.Background())
	if err != nil || profile.ID != 42 {
		t.Fatal(profile, err)
	}
	restored.http.Jar.SetCookies(origin, []*http.Cookie{{Name: "domain-session", Value: "secret", Path: "/", Domain: ".music.163.com"}})
	if err = restored.Logout(); err != nil {
		t.Fatal(err)
	}
	if len(restored.http.Jar.Cookies(origin)) != 0 {
		t.Fatal("cookies retained")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("session retained")
	}
}
func TestFullPlaylistAndUnavailableMedia(t *testing.T) {
	calls := 0
	c := testClient(t, "", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/weapi/v6/playlist/detail":
			ids := make([]map[string]int, 205)
			for i := range ids {
				ids[i] = map[string]int{"id": i + 1}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "playlist": map[string]any{"trackIds": ids}})
		case "/weapi/v3/song/detail":
			calls++
			start, end := 1, 200
			if calls == 2 {
				start, end = 201, 205
			}
			songs := []map[string]any{}
			for i := end; i >= start; i-- {
				songs = append(songs, map[string]any{"id": i, "name": "歌", "dt": 125000, "ar": []map[string]string{{"name": "歌手"}}, "al": map[string]string{"name": "专辑"}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "songs": songs})
		case "/weapi/song/enhance/player/url":
			fmt.Fprint(w, `{"code":200,"data":[{"url":null}]}`)
		}
	})
	tracks, err := c.Tracks(context.Background(), 1)
	if err != nil || len(tracks) != 205 || calls != 2 {
		t.Fatal(len(tracks), calls, err)
	}
	for i, track := range tracks {
		if track.ID != int64(i+1) || track.Duration != 125 {
			t.Fatal("order or duration", track)
		}
	}
	if _, err = c.SongURL(context.Background(), 1); err == nil {
		t.Fatal("unavailable song accepted")
	}
}
func TestErrorsDoNotExposeResponseAndCancellation(t *testing.T) {
	c := testClient(t, "", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"code":301,"message":"secret-session"}`) })
	_, err := c.Playlists(context.Background(), 1)
	if err == nil || strings.Contains(err.Error(), "secret-session") {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = c.Account(ctx); err == nil {
		t.Fatal("cancelled request succeeded")
	}
}
