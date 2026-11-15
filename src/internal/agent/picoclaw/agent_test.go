package picoclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEmbeddedPicoClawKeepsSessionHistoryAndGenericTools(t *testing.T) {
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
		if !strings.Contains(string(body["tools"]), `"name":"exec"`) || !strings.Contains(string(body["tools"]), `"name":"read_file"`) {
			t.Error("generic tools missing")
		}
		mu.Lock()
		requests = append(requests, string(body["messages"]))
		mu.Unlock()
		if string(body["stream"]) == "true" {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			payload, _ := json.Marshal(map[string]any{
				"id": "stream", "choices": []map[string]any{{"index": 0, "delta": map[string]any{"content": "你好，这是 PicoClaw 的回复。"}, "finish_reason": nil}},
			})
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
			flusher.Flush()
			return
		}
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
		reply, err := a.Chat(ctx, text, "session-one", nil)
		if err != nil || !strings.Contains(reply, "PicoClaw") {
			t.Fatalf("reply=%q err=%v", reply, err)
		}
	}
	if _, err = a.Chat(ctx, "新对话", "session-two", nil); err != nil {
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

func TestChatStreamsReplyChunks(t *testing.T) {
	chunks := []string{"# 标题\n", "**", "加粗", "**\n- ", "第一条", "\n```go\n", "fmt.Println(\"hi\")", "\n```\n"}
	received := make(chan string, len(chunks))
	advance := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, chunk := range chunks {
			payload, _ := json.Marshal(map[string]any{
				"id": "stream", "choices": []map[string]any{{"index": 0, "delta": map[string]any{"content": chunk}, "finish_reason": nil}},
			})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
			select {
			case <-advance:
			case <-r.Context().Done():
				return
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer s.Close()
	a, err := New(t.TempDir(), s.URL, "local-test-key", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	done := make(chan error, 1)
	go func() {
		reply, err := a.Chat(ctx, "hi", "session-one", func(chunk string) { received <- chunk })
		if err == nil && reply != strings.Join(chunks, "") {
			err = fmt.Errorf("reply=%q", reply)
		}
		done <- err
	}()
	want := ""
	for _, chunk := range chunks {
		want += chunk
		select {
		case got := <-received:
			if got != want {
				t.Fatalf("streamed Markdown=%q, want %q", got, want)
			}
		case <-ctx.Done():
			t.Fatal("Markdown was buffered until the final response")
		}
		select {
		case err := <-done:
			t.Fatalf("chat completed before the server finished: %v", err)
		default:
		}
		advance <- struct{}{}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
