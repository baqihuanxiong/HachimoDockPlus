# PicoClaw 通用智能体扩展

智能体、工具调用循环和技能发现运行在开发板上；大模型及当前 ASR/TTS 仍调用已配置的服务。应用不注册特定家居、音乐或其他业务工具。新能力通过本地 CLI + 技能，或者 MCP 提供。

## 通过 SMB 管理

升级程序后，已有 SMB 安装会在下次正常启动时补齐 `agent/`，无需重装 Samba。共享 `HachimoConfig` 的目录为：

```text
config.json                 应用、模型与语音配置
secrets/                    服务密钥、MCP stdio 环境文件
agent/
  runtime.json              工具、工具轮次、输出预算、MCP 服务
  AGENT.md                  智能体说明及可选 allowlist
  SOUL.md                   回复风格
  USER.md                   可选用户偏好
  bin/                      本地 CLI，ARMv7 Linux 二进制或脚本
  skills/<name>/SKILL.md     技能说明
  skills/<name>/...          技能脚本、配置及参考文件
```

保存文件后，在设置 → 系统选择“应用配置并重启”。JSON、密钥、技能和 CLI 同属一个配置快照；验证失败或验证期间文件继续变化时保留当前版本。仅上传不生效；删除文件后应用配置，会同时移除其运行副本。重启前只做本地格式和文件检查，不执行上传的代码，不拨号 MCP，也不发送家居控制。

SMB 上传通常没有执行位。应用配置时为 `bin/` 文件及带 shebang 的脚本恢复 0700，其他文件为 0600。当前限制：单文件 16 MiB、全部 agent 文件 32 MiB、含目录最多 512 项、目录深度最多 12。忽略点开头的编辑器临时文件，拒绝符号链接和特殊文件。

运行副本位于 `apps.picoclaw.data_dir/workspace`。`sessions/`、`memory/`、`cli-state/` 不共享，不因配置更新删除。首次迁移保留用户已有的提示词、技能和 CLI，只替换旧版本程序生成的原样“仅聊天”提示词。用户编辑过的 `tools: []` 等限制继续生效，可在共享的 `AGENT.md` 中调整。

## 本地命令与技能

内置命令工具名称是上游的 `exec`。默认通过系统 `sh -c` 执行；本板使用 BusyBox shell，不假设安装 Bash、Python、Node.js 或 jq。需要 Bash 语法的 CLI/脚本需自行安装解释器。POSIX shell 脚本使用 `#!/bin/sh`。

`exec` 子命令的 PATH 自动加入运行工作区的 `bin/`，工作目录默认是工作区。使用相对路径或标准 CLI 名称即可，不写死设备 IP 或安装版本。子命令还获得 `HACHIMODOCK_CLI_STATE_DIR`，指向工作区中的私有 `cli-state/`；CLI 可在其中创建自己的账号令牌和状态目录。没有修改 Go 进程全局 PATH。

例如上传 `agent/bin/demo-cli`：

```sh
#!/bin/sh
printf '{"status":"ok","message":"本地 CLI 已运行"}\n'
```

再上传 `agent/skills/demo/SKILL.md`：

```markdown
---
name: demo
description: 查询本机 demo 状态时使用。
---
通过 exec 调用 demo-cli，按返回的 JSON 回复，不编造结果。
```

技能 `name` 与目录名一致。技能列表进入模型上下文，模型可通过 `read_file` 按需读取完整技能。引用技能内资源时使用模型看到的实际技能路径。技能只是操作说明，仍需要已开放的文件和命令工具。

## runtime.json

首次启动生成完整默认配置。修改其中的字段，例如：

```json
{
  "max_tokens": 2048,
  "max_tool_iterations": 20,
  "context_window": 32768,
  "restrict_to_workspace": false,
  "tools": {
    "exec": {
      "enabled": true,
      "enable_deny_patterns": true,
      "allow_remote": true,
      "timeout_seconds": 60
    },
    "read_file": {"enabled": true, "max_read_file_size": 65536},
    "list_dir": {"enabled": true},
    "write_file": {"enabled": true},
    "edit_file": {"enabled": true},
    "append_file": {"enabled": true},
    "filter_sensitive_data": true,
    "mcp": {
      "enabled": true,
      "servers": {
        "remote": {
          "enabled": true,
          "type": "http",
          "url": "https://your-mcp-service.example/mcp",
          "headers": {"Authorization": "Bearer YOUR_TOKEN"}
        },
        "local": {
          "enabled": false,
          "type": "stdio",
          "command": "bin/example-mcp-server",
          "args": [],
          "env_file": "secrets/example-mcp.env"
        }
      }
    }
  }
}
```

`tools` 使用固定版本 PicoClaw 的 `ToolsConfig` 字段，可按需启用上游支持的工具。默认的 `allow_remote: true` 是因为本应用以自定义 `dock` channel 进入上游；它不对外开放网络聊天入口。`restrict_to_workspace` 和命令过滤是上游工具检查，不是 OS 沙箱；本板程序以 root 运行，上传 CLI 具有相同权限。

MCP 的 `http` / `streamable-http` 为 Streamable HTTP；`sse` 使用当前上游 MCP SDK 的 SSE 模式。`stdio.command` 含相对路径时相对于 `agent/`，运行时映射到工作区；裸命令使用系统 PATH。`env_file` 的相对路径以主 `config.json` 所在目录为基准。配置中的环境变量及 header 值为字面值，不做 `${...}` 替换。

MCP 第一次对话时才连接，连接初始化上限 30 秒，取消会中断初始化；成功后连接归智能体实例持有，跨对话轮次保留，应用关闭或程序重启时释放。本地 stdio 进程不会因第一轮结束而退出。远程服务不可达不会触发本地 CLI 替代执行。

大量远程工具可配置 `tools.mcp.discovery`：`enabled: true`，同时设置 `use_bm25: true` 或 `use_regex: true`；可配置 `ttl` 和 `max_search_results`。此时工具按需发现，单个服务可用 `deferred: false` 直接展示其工具。

`AGENT.md` 省略 `tools`、`skills`、`mcpServers` 时使用已配置的能力。可按上游语法列出工具或 MCP 服务名；`tools: []` 仍可明确关闭工具。`max_tool_iterations` 上限 100，当前应用单轮总时限仍为 3 分钟；命令默认 60 秒，CLI 自己应对网络请求设置超时。

## 可选米家扩展

见 [mijia CLI](../extensions/mijia/README.md)。它是独立、无第三方 Go 依赖的 Linux CLI，直接连接小米云端，安装后与其他 CLI 使用相同扩展机制。PicoClaw 不需要知道它的设备 ID、接口、账号或命令格式。
