package audio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProxyForwardsByteRangesAndCloses(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://music.163.com/" {
			t.Error("missing media referrer")
		}
		http.ServeContent(w, r, "song.wav", time.Time{}, strings.NewReader("0123456789"))
	}))
	defer upstream.Close()
	address, closeProxy, err := streamProxy(context.Background(), upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", address, nil)
	req.Header.Set("Range", "bytes=4-7")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 206 || string(body) != "4567" || res.Header.Get("Content-Range") != "bytes 4-7/10" {
		t.Fatal(res.StatusCode, string(body), res.Header)
	}
	closeProxy()
	if res, err = http.Get(address); err == nil {
		res.Body.Close()
		t.Fatal("loopback server retained")
	}
}
