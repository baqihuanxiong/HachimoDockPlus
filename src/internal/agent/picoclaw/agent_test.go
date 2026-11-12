package picoclaw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEmbeddedPicoClawKeepsSessionHistoryAndNoTools(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected model path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer local-test-key" {
			t.Error("missing model auth")
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if string(body["model"]) != `"test-model"` {
			t.Errorf("wrong model: %s", body["model"])
		}
		if v := string(body["tools"]); v != "" && v != "[]" && v != "null" {
			t.Errorf("voice profile exposed tools: %s", v)
		}
		mu.Lock()
		requests = append(requests, string(body["messages"]))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"test","choices":[{"index":0,"message":{"role":"assistant","content":"你好，这是 PicoClaw 的回复。"},"finish_reason":"stop"}],"usage":{"prompt_tokens":30,"completion_tokens":10,"total_tokens":40}}`))
	}))
	defer s.Close()
	a, err := New(t.TempDir(), s.URL, "local-test-key", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, text := range []string{"记住暗号小鸭", "还记得吗"} {
		reply, err := a.Chat(ctx, text, "session-one")
		if err != nil || !strings.Contains(reply, "PicoClaw") {
			t.Fatalf("reply=%q err=%v", reply, err)
		}
	}
	if _, err = a.Chat(ctx, "新对话", "session-two"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 3 {
		t.Fatalf("expected three model requests, got %d", len(requests))
	}
	if !strings.Contains(requests[1], "记住暗号小鸭") || !strings.Contains(requests[1], "还记得吗") {
		t.Fatal("PicoClaw lost session context")
	}
	if strings.Contains(requests[2], "记住暗号小鸭") {
		t.Fatalf("new session retained old history: %s", requests[2])
	}
}
