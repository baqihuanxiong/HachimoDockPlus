package picoclaw

import (
	"context"
	"encoding/json"
	"fmt"
	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"hachimodockplus/internal/agent/profile"
)

func writeAssets(t *testing.T, dir string, extra map[string][]byte) {
	t.Helper()
	files := profile.Defaults()
	for k, v := range extra {
		files[k] = v
	}
	if err := profile.Write(dir, files); err != nil {
		t.Fatal(err)
	}
}

// The model makes a real tool call, then consumes the actual tool result.
func toolModel(t *testing.T, toolName, arguments, want string) *httptest.Server {
	t.Helper()
	var rounds atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
			Tools json.RawMessage `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		var delta any
		finish := "stop"
		if rounds.Add(1)%2 == 1 {
			if !strings.Contains(string(body.Tools), `"name":"`+toolName+`"`) {
				t.Errorf("tool %s missing", toolName)
			}
			delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call_1", "type": "function", "function": map[string]string{"name": toolName, "arguments": arguments}}}}
			finish = "tool_calls"
		} else {
			found := false
			for _, m := range body.Messages {
				if m.Role == "tool" && strings.Contains(fmt.Sprint(m.Content), want) {
					found = true
				}
			}
			if !found {
				t.Errorf("actual tool result %q missing", want)
			}
			delta = map[string]string{"content": "工具执行完成"}
		}
		payload, _ := json.Marshal(map[string]any{"id": "stream", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
	}))
}

func TestSkillAndUploadedCLIExecuteThroughModel(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent's workspace")
	ext := filepath.Join(dir, "extensions")
	writeAssets(t, ext, map[string][]byte{
		"skills/demo/SKILL.md": []byte("---\nname: demo\ndescription: Use demo-cli for the local demo\n---\nRun demo-cli to inspect the demo.\n"),
		"bin/demo-cli":         []byte("#!/bin/sh\nprintf '{\"result\":\"uploaded-cli-ran\"}\\n'\n"),
	})
	// Simulate SMB's non-executable upload permissions.
	os.Chmod(filepath.Join(ext, "bin/demo-cli"), 0600)
	s := toolModel(t, "exec", `{"action":"run","command":"demo-cli"}`, "uploaded-cli-ran")
	defer s.Close()
	a, err := New(dir, s.URL, "test-key", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	definition := a.loop.GetRegistry().GetDefaultAgent()
	if !strings.Contains(definition.ContextBuilder.BuildSystemPrompt(), "demo") {
		t.Fatal("uploaded skill not discoverable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err = a.Chat(ctx, "运行 demo", "cli", nil); err != nil {
		t.Fatal(err)
	}
	// Private CLI state and session memory survive an extension update.
	state := filepath.Join(dir, "workspace/cli-state/keep")
	os.WriteFile(state, []byte("private"), 0600)
	a.Close()
	os.Remove(filepath.Join(ext, "bin/demo-cli"))
	b, err := New(dir, s.URL, "test-key", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if _, err = os.Stat(filepath.Join(dir, "workspace/bin/demo-cli")); !os.IsNotExist(err) {
		t.Fatal("deleted CLI remains active")
	}
	if data, _ := os.ReadFile(state); string(data) != "private" {
		t.Fatal("private state lost")
	}
}

func TestCommandPatternsMatchOriginalAndCancellationStopsChildren(t *testing.T) {
	dir := t.TempDir()
	cfg := pcconfig.DefaultConfig()
	cfg.Tools.Exec.CustomDenyPatterns = []string{`^printf`}
	cfg.Tools.Exec.CustomAllowPatterns = []string{`^printf allowed`}
	executor, err := newCLIExec(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	blocked := executor.Execute(context.Background(), map[string]any{"action": "run", "command": "printf denied"})
	if !blocked.IsError {
		t.Fatal("anchored deny ignored after PATH prefix")
	}
	allowed := executor.Execute(context.Background(), map[string]any{"action": "run", "command": "printf allowed"})
	if allowed.IsError || !strings.Contains(allowed.ForLLM, "allowed") {
		t.Fatal("explicit allow ignored", allowed)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	result := executor.Execute(ctx, map[string]any{"action": "run", "command": "sleep 30"})
	if !result.IsError || time.Since(started) > 3*time.Second {
		t.Fatal("shell process did not cancel")
	}
}

func TestRemoteMCPExecutesAndSurvivesTurnCancellation(t *testing.T) {
	var calls atomic.Int32
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "Echo a value"}, func(ctx context.Context, req *mcp.CallToolRequest, in struct {
		Text string `json:"text"`
	}) (*mcp.CallToolResult, any, error) {
		calls.Add(1)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "remote-" + in.Text}}}, nil, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-mcp-key" {
			t.Error("MCP credential missing")
			w.WriteHeader(401)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer remote.Close()
	dir := t.TempDir()
	files := profile.Defaults()
	var runtime map[string]any
	json.Unmarshal(files["runtime.json"], &runtime)
	runtime["tools"].(map[string]any)["mcp"] = map[string]any{"enabled": true, "servers": map[string]any{"demo": map[string]any{"enabled": true, "type": "http", "url": remote.URL, "headers": map[string]string{"Authorization": "Bearer test-mcp-key"}}}}
	files["runtime.json"], _ = json.Marshal(runtime)
	if err := profile.Write(filepath.Join(dir, "extensions"), files); err != nil {
		t.Fatal(err)
	}
	s := toolModel(t, "mcp_demo_echo", `{"text":"hello"}`, "remote-hello")
	defer s.Close()
	a, err := New(dir, s.URL, "test-key", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for range 2 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err = a.Chat(ctx, "查询远程", "mcp", nil)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("remote tool was not invoked twice", calls.Load())
	}
}

func TestExplicitNoToolsProfileStillWorks(t *testing.T) {
	dir := t.TempDir()
	writeAssets(t, filepath.Join(dir, "extensions"), map[string][]byte{"AGENT.md": []byte("---\nname: Chat\ntools: []\nmcpServers: []\n---\n只聊天\n")})
	a, err := New(dir, "http://127.0.0.1:1", "key", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if len(a.loop.GetRegistry().GetDefaultAgent().Tools.List()) != 0 {
		t.Fatal("explicit tool restriction ignored")
	}
}

func TestMCPStdioHelper(t *testing.T) {
	if os.Getenv("DOCK_TEST_STDIO_MCP") != "1" {
		return
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "stdio", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "Echo"}, func(ctx context.Context, req *mcp.CallToolRequest, in struct {
		Text string `json:"text"`
	}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "stdio-" + in.Text}}}, nil, nil
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestStdioMCPHasInstanceLifetime(t *testing.T) {
	dir := t.TempDir()
	files := profile.Defaults()
	var runtime map[string]any
	json.Unmarshal(files["runtime.json"], &runtime)
	command, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	runtime["tools"].(map[string]any)["mcp"] = map[string]any{"enabled": true, "servers": map[string]any{"local": map[string]any{"enabled": true, "type": "stdio", "command": command, "args": []string{"-test.run=^TestMCPStdioHelper$"}, "env": map[string]string{"DOCK_TEST_STDIO_MCP": "1"}}}}
	files["runtime.json"], _ = json.Marshal(runtime)
	if err := profile.Write(filepath.Join(dir, "extensions"), files); err != nil {
		t.Fatal(err)
	}
	s := toolModel(t, "mcp_local_echo", `{"text":"hello"}`, "stdio-hello")
	defer s.Close()
	a, err := New(dir, s.URL, "test-key", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for range 2 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err = a.Chat(ctx, "local", "stdio", nil)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCustomLegacyAssetsArePreserved(t *testing.T) {
	dir := t.TempDir()
	if err := profile.Write(filepath.Join(dir, "workspace"), map[string][]byte{"AGENT.md": []byte("---\nname: Custom\ntools: [exec]\n---\n用户定制\n"), "bin/custom-cli": []byte("#!/bin/sh\ntrue\n"), "memory/MEMORY.md": []byte("keep")}); err != nil {
		t.Fatal(err)
	}
	a, err := New(dir, "http://127.0.0.1:1", "key", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if a.loop.GetRegistry().GetDefaultAgent().Definition.Agent.Frontmatter.Name != "Custom" {
		t.Fatal("custom prompt overwritten")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "workspace/memory/MEMORY.md")); string(data) != "keep" {
		t.Fatal("memory overwritten")
	}
}
