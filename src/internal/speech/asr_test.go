package speech

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func asrServer(t *testing.T, finish func(*websocket.Conn)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(401)
			return
		}
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, b, err := conn.ReadMessage()
		if err != nil || string(b) != "START" {
			return
		}
		conn.WriteJSON(map[string]string{"event": "started"})
		_, b, err = conn.ReadMessage()
		if err != nil || string(b) != "LANGUAGE:中文" {
			return
		}
		conn.WriteJSON(map[string]string{"event": "language_set"})
		finish(conn)
	}))
}

func TestASRUsesCumulativeSnapshotsAndConfirmedFinal(t *testing.T) {
	var total atomic.Int64
	s := asrServer(t, func(c *websocket.Conn) {
		for {
			kind, b, err := c.ReadMessage()
			if err != nil {
				return
			}
			if kind == websocket.BinaryMessage {
				total.Add(int64(len(b)))
				continue
			}
			if string(b) == "STOP" {
				break
			}
		}
		c.WriteMessage(websocket.TextMessage, []byte(`{"sentences":[{"text":"你好"}],"partial":"世界","is_final":false}`))
		c.WriteMessage(websocket.TextMessage, []byte(`{"sentences":[{"text":"你好"}],"partial":"世界！","is_final":false}`))
		c.WriteMessage(websocket.TextMessage, []byte(`{"sentences":[{"text":"你好世界！"}],"partial":"不应使用的临时结果","is_final":true}`))
		c.WriteJSON(map[string]string{"event": "stopped"})
	})
	defer s.Close()
	a := ASR{URL: "ws" + strings.TrimPrefix(s.URL, "http"), Key: "test-key", Language: "中文"}
	var previews []string
	text, err := a.Transcribe(context.Background(), bytes.NewReader(make([]byte, 6400)), func(s string) { previews = append(previews, s) })
	if err != nil {
		t.Fatal(err)
	}
	if text != "你好世界！" {
		t.Fatalf("unconfirmed/duplicated transcript: %q", text)
	}
	if total.Load() != 6400 {
		t.Fatalf("sent %d PCM bytes", total.Load())
	}
	if !reflect.DeepEqual(previews[:2], []string{"你好世界", "你好世界！"}) {
		t.Fatal(previews)
	}
}

func TestASRStoppedWithoutFinalIsFailure(t *testing.T) {
	s := asrServer(t, func(c *websocket.Conn) {
		for {
			_, b, err := c.ReadMessage()
			if err != nil {
				return
			}
			if string(b) == "STOP" {
				break
			}
		}
		c.WriteJSON(map[string]string{"event": "stopped"})
	})
	defer s.Close()
	a := ASR{URL: "ws" + strings.TrimPrefix(s.URL, "http"), Key: "test-key", Language: "中文"}
	if _, err := a.Transcribe(context.Background(), bytes.NewReader(nil), nil); err == nil {
		t.Fatal("missing final accepted")
	}
}

func TestASRCancelInterruptsCaptureAndSocket(t *testing.T) {
	s := asrServer(t, func(c *websocket.Conn) {
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer s.Close()
	a := ASR{URL: "ws" + strings.TrimPrefix(s.URL, "http"), Key: "test-key", Language: "中文"}
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := a.Transcribe(ctx, reader, nil)
	if err == nil || time.Since(start) > time.Second {
		t.Fatal("cancel did not interrupt live capture", err)
	}
}
