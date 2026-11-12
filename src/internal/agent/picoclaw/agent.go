// Package picoclaw is the sole boundary to the pinned upstream agent API.
package picoclaw

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	pcagent "github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type Agent struct {
	mu   sync.Mutex
	loop *pcagent.AgentLoop
	bus  *bus.MessageBus
}

func New(dataDir, baseURL, key, model string) (*Agent, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	cfg := pcconfig.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(dataDir, "workspace")
	cfg.Agents.Defaults.ModelName = "dock-model"
	cfg.Agents.Defaults.MaxTokens = 1024
	cfg.Agents.Defaults.MaxToolIterations = 4
	cfg.Agents.Defaults.MaxParallelTurns = 1
	cfg.Agents.Defaults.MaxLLMRetries = 1
	cfg.Agents.Defaults.SummarizeMessageThreshold = 12
	cfg.Hooks.Enabled = false
	// Voice-chat profile: tools can be deliberately added by a later app.
	// Do not expose shell, peripheral or network tools to casual speech input.
	cfg.Tools = pcconfig.ToolsConfig{FilterSensitiveData: true, FilterMinLength: 8}
	cfg.ModelList = []*pcconfig.ModelConfig{{ModelName: "dock-model", Provider: "openai", Model: model, APIBase: baseURL, APIKeys: pcconfig.SecureStrings{pcconfig.NewSecureString(key)}, Enabled: true}}
	if err := os.MkdirAll(cfg.Agents.Defaults.Workspace, 0700); err != nil {
		return nil, err
	}
	definition := filepath.Join(cfg.Agents.Defaults.Workspace, "AGENT.md")
	if _, err := os.Stat(definition); os.IsNotExist(err) {
		if err = os.WriteFile(definition, []byte("---\nname: PicoClaw\ndescription: 本机语音对话助手\ntools: []\nskills: []\nmcpServers: []\n---\n你是 HachimoDock 上的 PicoClaw 语音助手。仅提供对话和建议，不声称执行不存在的工具。\n"), 0600); err != nil {
			return nil, err
		}
	}
	prompt := filepath.Join(cfg.Agents.Defaults.Workspace, "SOUL.md")
	if _, err := os.Stat(prompt); os.IsNotExist(err) {
		if err = os.WriteFile(prompt, []byte("你是 HachimoDock 上的 PicoClaw 语音助手。默认用简洁、自然的中文回答。回答用于屏幕显示和语音播报，不使用表格或复杂 Markdown。你当前没有硬件控制、文件操作或命令执行能力，不要声称已经执行这些操作。\n"), 0600); err != nil {
			return nil, err
		}
	}
	logger.SetLevel(logger.ERROR)
	provider := providers.NewHTTPProvider(key, baseURL, "")
	messageBus := bus.NewMessageBus()
	loop := pcagent.NewAgentLoop(cfg, messageBus, provider)
	if names := loop.GetRegistry().GetDefaultAgent().Tools.List(); len(names) != 0 {
		loop.Close()
		messageBus.Close()
		return nil, errors.New("当前语音应用要求 AGENT.md 使用 tools: []，请检查工作区配置")
	}
	return &Agent{loop: loop, bus: messageBus}, nil
}

func (a *Agent) Chat(ctx context.Context, text, session string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Upstream only treats opaque/agent-prefixed keys as explicit. A plain
	// string passed to ProcessDirect is otherwise replaced by cli/direct scope.
	reply, err := a.loop.ProcessDirectWithChannel(ctx, text, "agent:main:dock:"+session, "dock", session)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errors.New("PicoClaw 请求失败，请检查网络或模型配置")
	}
	if reply == "" {
		return "", errors.New("PicoClaw 返回空回复")
	}
	return reply, nil
}

func (a *Agent) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.loop.Stop()
	a.loop.Close()
	a.bus.Close()
}
