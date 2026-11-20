# HachimoDock Go + LVGL

MyHachimoDock 的 Linux 应用层，当前运行于 **Lyra Zero W / RK3506B（ARMv7、Buildroot、glibc）**，使用 Go 处理业务与网络、LVGL 绘制 640×480 横屏界面。板级固件与驱动见 [板级说明](../board/lyra-zero-w/README.md)。

内置组件如下，操作、配置和限制在各自文档中维护：

公共屏保支持时钟、纯黑和[桌宠](references/pet-screensaver.md)。桌宠使用 SMB 离线导入的 MP4，并可通过官方 Desktop 的 Wi-Fi 路径接收 Agent 状态；不实现 USB 或网络素材下发。

| 组件 ID | 功能与参考 |
|---|---|
| `picoclaw` | [文字与按键语音对话](references/picoclaw.md)，支持[技能、CLI 和 MCP 扩展](references/picoclaw-extensions.md)及[豆包语音](references/doubao-voice.md) |
| `music` | [网易云音乐](references/music.md)：短信登录、歌单、封面、歌词与播放控制 |
| `newapi-monitor` | [NewAPI 监控](references/newapi-monitor.md)：用量看板、上游余额与用户余额重置 |
| `settings` | [设置与公共界面](references/settings.md)：无线、外观、音频与配置管理 |

## 代码入口

应用采用**编译期注册、单进程运行**，首次打开时初始化。离开应用取消前台任务，实例保留业务状态；页面随进入、离开创建和销毁。后台任务通过状态快照更新页面，所有 LVGL 调用集中在一个锁定的 OS 线程。

| 位置 | 职责 |
|---|---|
| [`cmd/dock`](cmd/dock/main.go) | 启动参数、UI 选择及服务诊断 |
| [`internal/bootstrap`](internal/bootstrap/apps.go) | 应用工厂、共享服务、页面和终端命令的装配入口 |
| [`internal/core`](internal/core/) | 应用契约、注册、生命周期、导航和事件分发 |
| [`internal/apps`](internal/apps/) | 各组件的业务状态、配置及任务取消 |
| [`internal/ui`](internal/ui/) | LVGL 公共界面、`page_*` 页面适配器和终端前端 |
| [`internal/platform`](internal/platform/) | 音频、无线、按键、摇杆、电池和系统服务 |
| [`internal/agent`](internal/agent/) / [`internal/speech`](internal/speech/) | PicoClaw v0.3.1 适配及 ASR / TTS 客户端 |
| [`internal/config`](internal/config/) / [`preferences`](internal/preferences/) / [`filetree`](internal/filetree/) | 配置解析、外观偏好及配置资源快照 |
| [`tools`](tools/) / [`tests`](tests/) / [`patches`](patches/README.md) | 构建部署、原生 UI 测试及固定版本 LVGL 补丁 |

开始开发前阅读 [开发参考索引](references/README.md)。新增组件遵循 [应用开发规范](references/app-development.md)，在 `bootstrap/apps.go` 注册；硬件能力复用平台层，通常无需修改公共核心、目录或构建脚本。

## 本地构建与运行

开发主机需要 Docker、Python 3，以及可用的网络；部署另需 SSH / SCP。以下命令均从 `src/` 执行。`dev.sh` 自动构建开发镜像，固定 Go 1.25.11、Debian Bookworm ARM hard-float 工具链与 LVGL v9.3.0，无需在板端安装编译器。

```sh
tools/dev.sh tools/bootstrap.sh
```

`bootstrap.sh` 准备 LVGL、中文 / Emoji 字体、应用图标及参考音频，资源来源和许可随文件保存。然后准备配置，任选一种方式：

- 已有本地 `references/secret.md`：执行 `python3 tools/import_references.py`，生成 `.local/config.json` 和权限为 `600` 的密钥文件。**会覆盖已生成的本机配置**，只在首次导入或明确需要重新生成时执行。
- 手工配置：参考 [`config.example.json`](config.example.json) 创建 `.local/config.json`，填写所需组件的地址、模型与密钥。路径相对于配置文件目录；示例中的 `.local/assets/...`、`.local/secrets/...` 和 `.local/data` 应分别改为 `assets/...`、`secrets/...` 和 `data`。

示例服务地址与模型名是占位值；导入脚本只填充 PicoClaw 服务，NewAPI 等组件需另行配置。不使用的组件可移除其 `apps.<id>` 配置，目录入口仍会保留。

组件配置位于 `apps.<id>`，服务地址和密钥在打开相应组件时校验；只预览目录无需配置模型服务。应用数据默认位于 `data_dir/apps/<id>/`，外观偏好位于 `data_dir/preferences.json`。板端配置更新见 [SMB 配置共享](references/configuration-sharing.md)。

```sh
tools/dev.sh tools/build.sh native
tools/dev.sh tools/build.sh arm

# 离屏预览，不需要显示或触摸设备；-page 可指定组件 ID
tools/dev.sh build/native/dock -config .local/config.json -preview build/catalog.png

# 终端诊断；PicoClaw 支持普通文字及 /voice、/stop、/new
tools/dev.sh build/native/dock -config .local/config.json -ui terminal -page picoclaw
```

产物为 `build/native/dock` 和 `build/arm/dock`。ARM 产物动态依赖 glibc，LVGL 静态链入；musl 系统需另行适配工具链。

## 部署与服务

先确认开发板已经完成[硬件适配](references/hardware-adaptation.md)，将下面地址替换为实际 SSH 地址：

```sh
board_host=root@192.168.5.156
tools/deploy.sh "$board_host" --no-backup
# 首次安装，或更新了开机服务脚本时执行；安装后立即启动
tools/install-autostart.sh "$board_host"
```

程序安装到 `/opt/hachimodock/releases/<内容哈希>/dock`，`current` 指向当前版本。部署保留已有配置、密钥、数据和资源，只补充缺失资源；新增组件的配置与密钥须单独安装。已运行的服务会在切换版本前停止、随后恢复。`--no-backup` 删除旧应用版本；省略则保留。空间不足时脚本会在停止服务前退出。

```sh
ssh "$board_host" '/etc/init.d/S99hachimodock status'
ssh "$board_host" 'tail -n 40 /var/log/hachimodock.log'
```

服务支持 `start|stop|restart|status`，主进程退出后等待 3 秒重启；日志保留本次与上次运行（`.log.1`）。**手动运行 LVGL 或独占设备试验前先停服务**，结束后恢复，避免多个进程争用屏幕与硬件。部署版本核验、手动诊断与恢复步骤见 [实机闭环测试](references/board-validation.md)。

板端无可靠 RTC，时间错误会导致 HTTPS / WSS 校验失败。可执行 `ssh "$board_host" '/opt/hachimodock/current/dock -sync-time'`；没有其他 NTP 服务时，在 `/etc/default/hachimodock` 设置 `DOCK_SYNC_TIME=1` 并重启应用服务，启用后台 SNTP 重试，不阻塞界面。安装脚本默认值为 `0`，已有配置会保留；日志为 `/var/log/hachimodock-time.log`。

## 验证与维护

```sh
tools/dev.sh sh -c 'go test -race -p 2 -tags stdjson,goolm ./... && go vet -tags stdjson,goolm ./...'
# 先完成 native 构建，确保链接当前 LVGL 静态库
tools/dev.sh sh tools/check-ui.sh
```

Go 测试检查业务、架构边界、取消与资源清理；原生 UI 测试使用真实 LVGL 控件。预览和自动测试不能代替硬件验收，交付按 [实机闭环测试](references/board-validation.md) 执行，并用 [报告模板](references/templates/board-test-report.md) 记录证据与未验证项。

`.local/`、`.cache/`、`build/`、`references/secret.md` 已被 Git 忽略。凭据、账号信息、对话与原始采集文件不得写入公开文档或提交；资源来源与许可见 [应用图标说明](assets/app-icons/README.md) 及 `bootstrap.sh` 生成的 `.local/assets/`。
