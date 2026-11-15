package picoclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestToolRoundCommentaryRetainedSeparatelyWithNewlines(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stage := int(calls.Add(1)) - 1
		w.Header().Set("Content-Type", "text/event-stream")
		write := func(delta any, finish any) {
			payload, _ := json.Marshal(map[string]any{"id": "progress", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			w.(http.Flusher).Flush()
		}
		if stage < 2 {
			comment := []string{"读取本地技能\r\n检查工具能力", "获取状态\n整理查询结果"}[stage]
			write(map[string]string{"content": comment, "reasoning_content": "private-reasoning-sentinel"}, nil)
			write(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("tool_%d", stage), "type": "function", "function": map[string]string{"name": "exec", "arguments": `{"action":"run","command":"printf checked"}`}}}}, "tool_calls")
		} else {
			for _, text := range []string{"最终答复", "\n", "第二行"} {
				write(map[string]string{"content": text}, nil)
			}
			write(map[string]any{}, "stop")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer s.Close()
	a, err := New(t.TempDir(), s.URL, "key", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	var answer, process string
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reply, err := a.ChatWithProgress(ctx, "查询本地状态", "progress", func(text, steps string) {
		answer, process = text, steps
		if strings.Contains(text+steps, "private-reasoning-sentinel") {
			t.Error("private reasoning was exposed")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "最终答复\n第二行" || answer != reply {
		t.Fatalf("answer=%q reply=%q", answer, reply)
	}
	if process != "读取本地技能\n检查工具能力\n\n获取状态\n整理查询结果" {
		t.Fatalf("process lost text or line breaks: %q", process)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected two tool rounds then a final answer, got %d", calls.Load())
	}
}
