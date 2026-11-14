package audio

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"time"
)

// FFmpeg on the board supports HTTP, but not TLS. A private loopback bridge
// keeps HTTPS in Go and forwards byte ranges for demuxer seeking, without files.
func streamProxy(parent context.Context, address string) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	token := make([]byte, 16)
	if _, err = rand.Read(token); err != nil {
		listener.Close()
		return "", nil, err
	}
	path := "/" + hex.EncodeToString(token)
	ctx, cancel := context.WithCancel(parent)
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 10 * time.Second}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		requestCtx, stopRequest := context.WithCancel(r.Context())
		stopParent := context.AfterFunc(ctx, stopRequest)
		defer stopParent()
		defer stopRequest()
		req, err := http.NewRequestWithContext(requestCtx, r.Method, address, nil)
		if err != nil {
			http.Error(w, "media request failed", 502)
			return
		}
		req.Header.Set("Referer", "https://music.163.com/")
		for _, name := range []string{"Range", "If-Range"} {
			if value := r.Header.Get(name); value != "" {
				req.Header.Set(name, value)
			}
		}
		res, err := mediaHTTP.Do(req)
		if err != nil {
			http.Error(w, "media unavailable", 502)
			return
		}
		defer res.Body.Close()
		for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
			if value := res.Header.Get(name); value != "" {
				w.Header().Set(name, value)
			}
		}
		w.WriteHeader(res.StatusCode)
		if r.Method == http.MethodGet {
			_, _ = io.Copy(w, res.Body)
		}
	})
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(listener) }()
	closeProxy := func() { cancel(); _ = server.Close(); <-done }
	return "http://" + listener.Addr().String() + path, closeProxy, nil
}
