// Package picoclaw is the sole boundary to the pinned upstream agent API.
package picoclaw

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	pcagent "github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/mcp"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type Agent struct {
	mu        sync.Mutex
	loop      *pcagent.AgentLoop
	bus       *bus.MessageBus
	stream    *streamSink
	closed    bool
	mcpConfig pcconfig.MCPConfig
	mcp       *mcp.Manager
	mcpCancel context.CancelFunc
	life      context.Context
	stop      context.CancelFunc
}

// streamSink separates visible tool-round commentary from the final answer.
// Upstream cancels a draft before executing tools, then starts a new accumulated
// answer stream. ReasoningContent is intentionally not forwarded here.
type streamSink struct {
	chatID   string
	onUpdate func(string, string)
	text     string
	process  string
}

func (s *streamSink) Update(_ context.Context, content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	s.text = strings.ReplaceAll(content, "\r\n", "\n")
	s.emit()
	return nil
}
func (s *streamSink) Finalize(_ context.Context, content string) error {
	if strings.TrimSpace(content) != "" {
		s.text = strings.ReplaceAll(content, "\r\n", "\n")
		s.emit()
	}
	return nil
}
func (s *streamSink) emit() {
	if s.onUpdate != nil {
		s.onUpdate(s.text, s.process)
	}
}
func (s *streamSink) Cancel(context.Context) {
	if text := strings.TrimSpace(s.text); text != "" {
		if s.process != "" {
			s.process += "\n\n"
		}
		s.process += text
		// Bound per-message UI history while retaining the newest stages.
		const limit = 64 << 10
		if len(s.process) > limit {
			start := len(s.process) - limit
			for start < len(s.process) && (s.process[start]&0xc0) == 0x80 {
				start++
			}
			s.process = "…\n" + s.process[start:]
		}
		s.text = ""
		s.emit()
	}
}

// dockChannelSettings lets the upstream channel-streaming check decode our
// synthetic "dock" channel settings.
type dockChannelSettings struct {
	Streaming pcconfig.StreamingConfig `json:"streaming,omitzero"`
}

func init() { pcconfig.RegisterChannelSettings("dock", dockChannelSettings{}) }

// streamDelegate exposes the active sink to the upstream pipeline. It is only
// consulted from within Agent.Chat while a.mu is held, so it reads the sink
// without taking the lock.
type streamDelegate struct{ agent *Agent }

func (d streamDelegate) GetStreamer(_ context.Context, _, chatID, _ string) (bus.Streamer, bool) {
	if d.agent.stream != nil && d.agent.stream.chatID == chatID {
		return d.agent.stream, true
	}
	return nil, false
}

func New(dataDir, baseURL, key, model string, options ...Options) (*Agent, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	cfg := pcconfig.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(dataDir, "workspace")
	cfg.Agents.Defaults.ModelName = "dock-model"
	cfg.Agents.Defaults.MaxTokens = 1024
	cfg.Agents.Defaults.MaxParallelTurns = 1
	cfg.Agents.Defaults.MaxLLMRetries = 1
	cfg.Agents.Defaults.SummarizeMessageThreshold = 12
	cfg.Hooks.Enabled = false
	cfg.ModelList = []*pcconfig.ModelConfig{{
		ModelName: "dock-model", Provider: "openai", Model: model, APIBase: baseURL,
		APIKeys: pcconfig.SecureStrings{pcconfig.NewSecureString(key)}, Enabled: true,
		Streaming: pcconfig.ModelStreamingConfig{Enabled: true},
	}}
	// Upstream only streams when both the model and the inbound channel opt in.
	cfg.Channels = pcconfig.ChannelsConfig{
		"dock": {
			Enabled: true, Type: "dock",
			Settings: pcconfig.RawNode(`{"streaming":{"enabled":true}}`),
		},
	}
	if err := os.MkdirAll(cfg.Agents.Defaults.Workspace, 0700); err != nil {
		return nil, err
	}
	var opts Options
	if len(options) > 0 {
		opts = options[0]
	}
	if err := prepareExtensions(dataDir, opts, cfg); err != nil {
		return nil, err
	}
	// Keep upstream fallback/anomaly warnings visible; info/debug stay off.
	logger.SetLevel(logger.WARN)
	provider := providers.NewHTTPProvider(key, baseURL, "")
	messageBus := bus.NewMessageBus()
	life, stop := context.WithCancel(context.Background())
	agent := &Agent{mcpConfig: cfg.Tools.MCP, life: life, stop: stop}
	// MCP connections belong to the app instance. Upstream initializes stdio
	// with the first turn's context, which would kill servers when that turn ends.
	// Reuse its manager and tool wrappers with an explicit instance lifetime.
	cfg.Tools.MCP.Enabled = false
	messageBus.SetStreamDelegate(streamDelegate{agent})
	loop := pcagent.NewAgentLoop(cfg, messageBus, provider)
	registry := loop.GetRegistry().GetDefaultAgent().Tools
	if _, ok := registry.Get("exec"); ok {
		execTool, err := newCLIExec(cfg.Agents.Defaults.Workspace, cfg)
		if err != nil {
			stop()
			loop.Close()
			messageBus.Close()
			return nil, err
		}
		registry.Register(execTool)
	}
	agent.loop, agent.bus = loop, messageBus
	return agent, nil
}

func (a *Agent) Chat(ctx context.Context, text, session string, onChunk func(string)) (string, error) {
	return a.ChatWithProgress(ctx, text, session, func(reply, _ string) {
		if onChunk != nil {
			onChunk(reply)
		}
	})
}

// ChatWithProgress reports accumulated answer text and retained public
// commentary separately. Only the returned final answer should be spoken.
func (a *Agent) ChatWithProgress(ctx context.Context, text, session string, onUpdate func(string, string)) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return "", errors.New("智能体已关闭")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := a.ensureMCP(ctx); err != nil {
		return "", err
	}
	a.stream = &streamSink{chatID: session, onUpdate: onUpdate}
	defer func() { a.stream = nil }()
	// Upstream only treats opaque/agent-prefixed keys as explicit. A plain
	// string passed to ProcessDirect is otherwise replaced by cli/direct scope.
	reply, err := a.loop.ProcessDirectWithChannel(ctx, text, "agent:main:dock:"+session, "dock", session)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errors.New("PicoClaw 请求失败，请检查模型、CLI 或 MCP 配置")
	}
	if reply == "" {
		return "", errors.New("PicoClaw 返回空回复")
	}
	return reply, nil
}

func (a *Agent) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	a.closed = true
	a.stop()
	if a.mcpCancel != nil {
		a.mcpCancel()
	}
	if a.mcp != nil {
		_ = a.mcp.Close()
	}
	a.loop.Stop()
	a.loop.Close()
	a.bus.Close()
}
