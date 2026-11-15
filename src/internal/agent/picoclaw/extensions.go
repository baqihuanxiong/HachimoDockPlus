package picoclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	pcagent "github.com/sipeed/picoclaw/pkg/agent"
	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/skills"
	"github.com/sipeed/picoclaw/pkg/tools"
	"gopkg.in/yaml.v3"
	"hachimodockplus/internal/agent/profile"
)

type Options struct{ ExtensionsDir string }

type runtimeConfig struct {
	MaxTokens           int                  `json:"max_tokens"`
	MaxToolIterations   int                  `json:"max_tool_iterations"`
	ContextWindow       int                  `json:"context_window,omitempty"`
	RestrictToWorkspace bool                 `json:"restrict_to_workspace"`
	Tools               pcconfig.ToolsConfig `json:"tools"`
}

func loadRuntime(dir, installedBase string) (runtimeConfig, error) {
	var c runtimeConfig
	files, err := profile.Read(dir)
	if err != nil {
		return c, err
	}
	data, ok := files["runtime.json"]
	if !ok {
		return c, errors.New("缺少 agent/runtime.json")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, errors.New("agent/runtime.json 格式错误：" + err.Error())
	}
	if d.Decode(new(any)) != io.EOF {
		return c, errors.New("agent/runtime.json 必须只包含一个对象")
	}
	if c.MaxTokens < 128 || c.MaxTokens > 32768 || c.MaxToolIterations < 1 || c.MaxToolIterations > 100 || (c.ContextWindow != 0 && c.ContextWindow < c.MaxTokens) {
		return c, errors.New("智能体输出预算或工具轮次无效")
	}
	if c.Tools.Exec.TimeoutSeconds < 0 || c.Tools.Exec.TimeoutSeconds > 180 {
		return c, errors.New("exec 超时需为 0 至 180 秒")
	}
	if c.Tools.MCP.Discovery.Enabled && !c.Tools.MCP.Discovery.UseBM25 && !c.Tools.MCP.Discovery.UseRegex {
		return c, errors.New("MCP 发现模式需开启 use_bm25 或 use_regex")
	}
	for _, patterns := range [][]string{c.Tools.AllowReadPaths, c.Tools.AllowWritePaths, c.Tools.Exec.CustomAllowPatterns, c.Tools.Exec.CustomDenyPatterns} {
		for _, pattern := range patterns {
			if _, err := regexp.Compile(pattern); err != nil {
				return c, errors.New("工具路径或命令正则表达式无效")
			}
		}
	}
	for name, server := range c.Tools.MCP.Servers {
		if strings.TrimSpace(name) == "" {
			return c, errors.New("MCP 名称不能为空")
		}
		if !server.Enabled {
			continue
		}
		if server.Deferred != nil && *server.Deferred && !c.Tools.MCP.Discovery.Enabled {
			return c, fmt.Errorf("MCP %s 的 deferred 需要启用发现模式", name)
		}
		switch pcconfig.EffectiveMCPTransportType(server) {
		case "stdio":
			if strings.TrimSpace(server.Command) == "" || server.URL != "" {
				return c, fmt.Errorf("MCP %s 的 stdio 配置无效", name)
			}
			// A relative executable containing a slash is rooted in the editable
			// profile; bare commands continue to resolve through the system PATH.
			if !filepath.IsAbs(server.Command) && strings.ContainsRune(server.Command, '/') {
				server.Command = filepath.Join(dir, server.Command)
			}
		case "http", "sse":
			u, err := url.Parse(server.URL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" || server.Command != "" {
				return c, fmt.Errorf("MCP %s 的 HTTP 地址无效", name)
			}
		default:
			return c, fmt.Errorf("MCP %s 的传输类型无效", name)
		}
		if server.EnvFile != "" && !filepath.IsAbs(server.EnvFile) {
			server.EnvFile = filepath.Join(installedBase, server.EnvFile)
		}
		c.Tools.MCP.Servers[name] = server
	}
	definition := pcagent.NewContextBuilder(dir).LoadAgentDefinition()
	if definition.Agent != nil && definition.Agent.FrontmatterErr != "" {
		return c, errors.New("agent/AGENT.md 元数据格式错误")
	}
	for name, data := range files {
		parts := strings.Split(filepath.ToSlash(name), "/")
		if len(parts) != 3 || parts[0] != "skills" || parts[2] != "SKILL.md" {
			continue
		}
		text := strings.ReplaceAll(string(data), "\r\n", "\n")
		var meta struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}
		end := strings.Index(strings.TrimPrefix(text, "---\n"), "\n---")
		if !strings.HasPrefix(text, "---\n") || end < 0 || yaml.Unmarshal([]byte(text[4:4+end]), &meta) != nil || skills.ValidateSkillName(meta.Name) != nil || meta.Name != parts[1] || strings.TrimSpace(meta.Description) == "" || len(meta.Description) > 1024 {
			return c, fmt.Errorf("技能 %s 的元数据无效，name 必须与目录名相同且包含 description", name)
		}
	}
	return c, nil
}

// ValidateExtensions performs no network requests and runs no uploaded code.
func ValidateExtensions(dir, installedBase string) error {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	_, err := loadRuntime(dir, installedBase)
	return err
}

// EnsureExtensions migrates user-authored workspace assets on first use. Only
// the exact old generated chat-only prompts are replaced with generic defaults.
func EnsureExtensions(dataDir string, opts Options) error {
	dir := opts.ExtensionsDir
	if dir == "" {
		dir = filepath.Join(dataDir, "extensions")
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	if _, err := os.Stat(filepath.Join(dir, "runtime.json")); errors.Is(err, os.ErrNotExist) {
		files, err := profile.Read(dir)
		if err != nil {
			return err
		}
		workspace := filepath.Join(dataDir, "workspace")
		legacyAgent := "---\nname: PicoClaw\ndescription: 本机语音对话助手\ntools: []\nskills: []\nmcpServers: []\n---\n你是 HachimoDock 上的 PicoClaw 语音助手。仅提供对话和建议，不声称执行不存在的工具。\n"
		legacySoul := "你是 HachimoDock 上的 PicoClaw 语音助手。默认用简洁、自然的中文回答。屏幕支持简单 Markdown（标题、列表、加粗、行内代码），可以使用；不要使用表格或图片。回答同时用于语音播报，保持朗读通顺。你当前没有硬件控制、文件操作或命令执行能力，不要声称已经执行这些操作。\n"
		for _, name := range []string{"AGENT.md", "SOUL.md", "USER.md"} {
			if _, exists := files[name]; exists {
				continue
			}
			path := filepath.Join(workspace, name)
			if st, err := os.Lstat(path); err == nil && st.Mode().IsRegular() && st.Size() <= profile.MaxFileBytes {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if (name == "AGENT.md" && string(data) == legacyAgent) || (name == "SOUL.md" && string(data) == legacySoul) {
					continue
				}
				files[name] = data
			}
		}
		for _, tree := range []string{"skills", "bin"} {
			assets, err := profile.Read(filepath.Join(workspace, tree))
			if err != nil {
				return err
			}
			for name, data := range assets {
				key := filepath.Join(tree, name)
				if _, exists := files[key]; !exists {
					files[key] = data
				}
			}
		}
		if err := profile.Write(dir, files); err != nil {
			return err
		}
	}
	return profile.Ensure(dir)
}

func prepareExtensions(dataDir string, opts Options, cfg *pcconfig.Config) error {
	dir := opts.ExtensionsDir
	if dir == "" {
		dir = filepath.Join(dataDir, "extensions")
	}
	if err := EnsureExtensions(dataDir, opts); err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	c, err := loadRuntime(resolved, filepath.Dir(dir))
	if err != nil {
		return err
	}
	workspace := cfg.Agents.Defaults.Workspace
	files, err := profile.Read(resolved)
	if err != nil {
		return err
	}
	// Only managed assets are replaced. sessions/, memory/ and cli-state/ remain
	// private and survive profile changes and restarts.
	for _, name := range []string{"skills", "bin"} {
		if err := os.RemoveAll(filepath.Join(workspace, name)); err != nil {
			return err
		}
	}
	managed := map[string][]byte{}
	for name, data := range files {
		if name == "runtime.json" {
			continue
		}
		if name == "AGENT.md" || name == "SOUL.md" || name == "USER.md" || strings.HasPrefix(filepath.ToSlash(name), "skills/") || strings.HasPrefix(filepath.ToSlash(name), "bin/") {
			managed[name] = data
		}
	}
	for _, name := range []string{"AGENT.md", "SOUL.md", "USER.md"} {
		if _, exists := managed[name]; !exists {
			if err := os.Remove(filepath.Join(workspace, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	if err := profile.Write(workspace, managed); err != nil {
		return err
	}
	for _, name := range []string{"bin", "skills", "cli-state"} {
		if err := os.MkdirAll(filepath.Join(workspace, name), 0700); err != nil {
			return err
		}
	}
	cfg.Agents.Defaults.MaxTokens = c.MaxTokens
	cfg.Agents.Defaults.MaxToolIterations = c.MaxToolIterations
	cfg.Agents.Defaults.ContextWindow = c.ContextWindow
	cfg.Agents.Defaults.RestrictToWorkspace = c.RestrictToWorkspace
	cfg.Tools = c.Tools
	for name, server := range cfg.Tools.MCP.Servers {
		if strings.HasPrefix(server.Command, resolved+string(filepath.Separator)) {
			rel, _ := filepath.Rel(resolved, server.Command)
			server.Command = filepath.Join(workspace, rel)
		}
		cfg.Tools.MCP.Servers[name] = server
	}
	return nil
}

// cliExec uses upstream execution and cancellation, adding a per-command PATH.
// It neither modifies the process environment nor names any particular CLI.
type cliExec struct {
	tools.Tool
	workspace   string
	allowed     tools.Tool
	allow, deny []*regexp.Regexp
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

func newCLIExec(workspace string, cfg *pcconfig.Config) (tools.Tool, error) {
	// User command patterns must match the original command, before our env
	// prefix. Keep upstream default guards, channel checks and path checks.
	copyCfg := *cfg
	copyCfg.Tools.Exec.CustomAllowPatterns = nil
	copyCfg.Tools.Exec.CustomDenyPatterns = nil
	var paths []*regexp.Regexp
	for _, pattern := range cfg.Tools.AllowReadPaths {
		paths = append(paths, regexp.MustCompile(pattern))
	}
	base, err := tools.NewExecToolWithConfig(workspace, cfg.Agents.Defaults.RestrictToWorkspace, &copyCfg, paths)
	if err != nil {
		return nil, err
	}
	t := cliExec{Tool: base, workspace: workspace}
	if cfg.Tools.Exec.EnableDenyPatterns {
		for _, pattern := range cfg.Tools.Exec.CustomDenyPatterns {
			t.deny = append(t.deny, regexp.MustCompile(pattern))
		}
	}
	for _, pattern := range cfg.Tools.Exec.CustomAllowPatterns {
		t.allow = append(t.allow, regexp.MustCompile(pattern))
	}
	if len(t.allow) > 0 {
		copyCfg.Tools.Exec.CustomAllowPatterns = []string{".*"}
		t.allowed, err = tools.NewExecToolWithConfig(workspace, cfg.Agents.Defaults.RestrictToWorkspace, &copyCfg, paths)
		if err != nil {
			return nil, err
		}
	}
	return t, nil
}

func (t cliExec) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	delegate := t.Tool
	copyArgs := make(map[string]any, len(args))
	for k, v := range args {
		copyArgs[k] = v
	}
	if command, ok := copyArgs["command"].(string); ok {
		if action, _ := args["action"].(string); action == "" || action == "run" {
			lower := strings.ToLower(strings.TrimSpace(command))
			allowed := false
			for _, pattern := range t.allow {
				if pattern.MatchString(lower) {
					allowed = true
					break
				}
			}
			if allowed {
				delegate = t.allowed
			} else {
				for _, pattern := range t.deny {
					if pattern.MatchString(lower) {
						return tools.ErrorResult("Command blocked by configured deny pattern")
					}
				}
			}
		}
		copyArgs["command"] = "export PATH=" + shellQuote(filepath.Join(t.workspace, "bin")) + ":\"$PATH\"; export HACHIMODOCK_CLI_STATE_DIR=" + shellQuote(filepath.Join(t.workspace, "cli-state")) + "; " + command
	}
	return delegate.Execute(ctx, copyArgs)
}
