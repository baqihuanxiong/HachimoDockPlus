package picoclaw

import (
	"context"
	"errors"
	"time"

	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/mcp"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// Called with Agent.mu held. Cancellation during connection aborts the attempt;
// after initialization, connections survive individual turn cancellation and
// are released by Close. No network activity occurs in the app constructor.
func (a *Agent) ensureMCP(turn context.Context) error {
	if !a.mcpConfig.Enabled || a.mcp != nil {
		return nil
	}
	instance := a.loop.GetRegistry().GetDefaultAgent()
	selected := a.mcpConfig
	selected.Servers = make(map[string]pcconfig.MCPServerConfig)
	for name, cfg := range a.mcpConfig.Servers {
		if cfg.Enabled && instance.AllowsMCPServer(name) {
			selected.Servers[name] = cfg
		}
	}
	if len(selected.Servers) == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(a.life)
	abort := context.AfterFunc(turn, cancel)
	timer := time.AfterFunc(30*time.Second, cancel)
	manager := mcp.NewManager()
	err := manager.LoadFromMCPConfig(ctx, selected, instance.Workspace)
	stoppedAbort, stoppedTimer := abort(), timer.Stop()
	if err != nil || !stoppedAbort || !stoppedTimer || turn.Err() != nil || ctx.Err() != nil {
		cancel()
		_ = manager.Close()
		if turn.Err() != nil {
			return turn.Err()
		}
		return errors.New("MCP 连接失败，请检查 agent/runtime.json 中的服务、凭据及网络")
	}
	// cancel is retained as a child of a.life; Close cancels it with the parent.
	for name, connection := range manager.GetServers() {
		deferred := selected.Discovery.Enabled
		if connection.Config.Deferred != nil {
			deferred = *connection.Config.Deferred
		}
		for _, capability := range connection.Tools {
			tool := tools.NewMCPTool(manager, name, capability)
			tool.SetWorkspace(instance.Workspace)
			tool.SetMaxInlineTextRunes(selected.GetMaxInlineTextChars())
			if deferred {
				instance.Tools.RegisterHidden(tool)
			} else {
				instance.Tools.Register(tool)
			}
		}
	}
	if selected.Discovery.Enabled {
		ttl := selected.Discovery.TTL
		if ttl <= 0 {
			ttl = 5
		}
		limit := selected.Discovery.MaxSearchResults
		if limit <= 0 {
			limit = 5
		}
		if selected.Discovery.UseRegex {
			instance.Tools.Register(tools.NewRegexSearchTool(instance.Tools, ttl, limit))
		}
		if selected.Discovery.UseBM25 {
			instance.Tools.Register(tools.NewBM25SearchTool(instance.Tools, ttl, limit))
		}
	}
	a.mcp = manager
	a.mcpCancel = cancel
	return nil
}
