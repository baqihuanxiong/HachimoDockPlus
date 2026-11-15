# 通过 SMB 编辑应用配置

各应用的低频业务配置保存在 `config.json` 的 `apps.<id>` 下，组件排序等公共界面选项放在 `ui` 下。例如 PicoClaw 的模型地址、模型名称和语音服务参数，无需在小屏幕上逐项输入。设置 → 系统提供音量、音频输出、SMB 开关、共享账号设置及“应用配置并重启”。

## 使用方法

1. 在设置 → 系统 → 共享账号中设置用户名、密码。首次保存成功会开启共享；没有预设密码，也不支持访客登录。用户名以小写字母开头，仅含小写字母、数字、`-`、`_`，最长 32 位；密码至少 8 个字符，最多 128 字节。
2. 电脑与开发板连入同一局域网。Windows 在资源管理器输入 `\\开发板IP\HachimoConfig`；macOS 连接服务器输入 `smb://开发板IP/HachimoConfig`，使用刚设置的共享账号。界面显示当前共享地址。
3. 编辑共享中的 `config.json`，密钥编辑 `secrets/` 中对应文件。PicoClaw 的通用工具与 MCP 配置位于 `agent/runtime.json`，提示词位于 `agent/AGENT.md`、`SOUL.md`，技能放入 `agent/skills/`，本地 CLI 放入 `agent/bin/`；见 [智能体扩展](picoclaw-extensions.md)。保存并等待编辑器完成写入。
4. 点击“应用配置并重启”并确认。程序校验配置及密钥，成功后切换生效配置，停止当前操作并退出；后台守护进程约 3 秒后重新启动 Go 程序。此操作不会重启 Linux。校验失败会显示错误，继续使用原配置，允许在共享中修正后重试。

共享内容是**待应用配置**。仅保存文件不会影响正在运行的组件。相对路径始终以 `/opt/hachimodock` 为基准；例如 `secrets/llm.key`、`assets/WenQuanYiMicroHei.ttc`。新增组件配置放入 `apps.<id>`，不要覆盖其他组件的字段。账号与密码独立于应用配置，不写入共享文件、界面状态快照或日志；换账号后，电脑可能需要断开原有 SMB 会话再登录。

共享包含配置、资源文件、服务密钥及用户安装的智能体扩展，只向需要编辑配置的可信电脑提供账号。共享范围仅为 `/opt/hachimodock/config-share`，不包含主程序、历史会话、私有 CLI 运行数据或 SMB 账号数据库。不跟随符号链接。服务要求 SMB 3、签名和传输加密；不兼容仅支持 SMB 1/2 的客户端。

## 组件排列顺序

编辑共享 `config.json` 中现有的 `ui` 对象，加入或修改 `app_order`，例如：

```json
"app_order": ["music", "picoclaw", "settings"]
```

这会按“网易云音乐 → PicoClaw → 设置”排列，位置为左上 → 右上 → 左下 → 右下，满四个进入下一页。当前内置组件 ID 为 `picoclaw`、`settings`、`music`，使用 ID 而不是中文标题。

- 按数组顺序优先放置列出的组件；只填 `["music"]` 也可以，其余组件按默认注册顺序追加，不会隐藏组件。
- 省略 `app_order` 或设置 `[]` 使用默认顺序（当前为 PicoClaw → 设置 → 网易云音乐）。新增组件自动追加。
- 重复 ID、未注册的 ID 或错误的数据类型会显示校验错误，保留原顺序且不重启。移除组件时也要从此数组删除其 ID。
- 保存后点击设置 → 系统 → 应用配置并重启。无需重新编译或部署程序。不要覆盖 `ui` 中原有字体、时区、触摸和摇杆设置。


## 安装与布局

先完成 ARM 构建、部署和 [开机服务安装](../README.md)，再构建、安装 Samba：

```sh
tools/dev.sh tools/build.sh arm
tools/deploy.sh root@192.168.4.202 --no-backup
sh tools/build-samba.sh
sh tools/install-smb.sh root@192.168.4.202
```

`install-smb.sh` 首次准备配置布局时停止 Go 服务，完成后恢复原来正在运行的服务。后续安装保留待应用的编辑、共享账号和数据。`--runtime-only` 仅供运行库维护，不准备配置布局。板端路径如下：

| 路径 | 用途 |
|---|---|
| `/opt/hachimodock/config-share/{config.json,secrets/,agent/,assets/}` | 远程编辑区，SMB 上传文件为 600、目录 700 |
| `/opt/hachimodock/config.json`、`secrets`、`agent`、`assets` | 稳定入口，指向同一生效配置代次 |
| `/opt/hachimodock/system/config-active` | 单一原子切换入口 |
| `/opt/hachimodock/system/config-versions/cfg-*` | 生效配置、密钥、扩展及资源；正常应用后清除旧代次，不累积备份 |
| `/opt/hachimodock/system/smb/` | SMB 用户名、密码哈希、配置和状态，不共享 |
| `/opt/hachimodock/samba/runtime.tgz` | NAND 上的压缩 Samba 运行库，约 27 MiB |
| `/tmp/hachimodock-samba-runtime` | 首次使用时解压到 RAM，约 65 MiB，重启后自动恢复 |
| `/etc/init.d/S98hachimodock-smb` | 独立 SMB 服务，`start/stop/restart/status` |
| `/var/log/hachimodock-smb.log` | Samba 诊断日志，上限约 128 KiB |

Samba 与 Go 程序独立，应用重启期间共享继续可用。关闭共享不会删除账号；再次开启无需重输密码。共享账号使用独立、禁止 shell 登录的本地账号，拒绝占用既有系统账号。

构建脚本从 Debian ARMHF 软件包收集动态依赖和许可记录，使用板端已有 glibc。版本与来源随包写入 `VERSION.txt`、`SOURCES.txt`，许可在 `licenses/`。库位数、glibc 兼容性、RAM 和 NAND 余量是换板后需要重新验证的条件。协议配置依据 [Samba smb.conf 官方文档](https://www.samba.org/samba/docs/current/man-html/smb.conf.5.html)。

## 新组件与校验

应用仍在自己的 `config.go` 中定义 schema 和校验。`internal/bootstrap/system.go` 负责重启前的组合校验：公共 JSON / 组件排序 / 字体 / 设备 / 时区，以及已配置的 PicoClaw 服务、密钥和参考音频。新增需要重启生效的组件时，把其配置校验接入此处，不在通用系统服务中导入应用包。

`config.Parse(data, installedBase)` 允许验证候选文件且保持安装路径语义；密钥校验使用待提交快照中的 `secrets`。校验不应拨号远程服务或发起付费请求，服务可用性由组件打开后的正常错误处理及实机测试验证。

系统服务先读取有限大小的普通文件，复制成完整代次、同步文件与目录，校验并确认远端文件没有继续变化，最后原子切换单个链接。主 JSON 上限 1 MiB，secrets 最多 64 个、单文件 64 KiB，支持 MCP 的多行 env 文件，密钥目录不支持子目录或链接；模型/语音的 key 文件仍要求非空单行。agent 单文件 16 MiB、总计 32 MiB，最多 512 项（含目录）、深度最多 12。应用时为 bin 文件和带 shebang 的脚本恢复 0700。运行副本、会话、记忆及 CLI 私有状态位于应用数据目录；不共享会话，也不会因配置切换删除私有状态。

PicoClaw 重启前会校验 runtime JSON、工具参数、MCP 传输地址、AGENT 元数据和技能元数据，不连接远程服务、不执行 CLI。MCP 和 CLI 的实际可用性在正常工具调用中验证。升级后首次正常启动自动补齐 agent 布局，不覆盖共享中尚未应用的编辑。

## 实机闭环要求

使用临时共享账号及权限为 600 的客户端认证文件；密码不放命令行、测试报告、截图或仓库。Linux 可用 `smbclient -A 认证文件 -m SMB3 --client-protection=encrypt` 连接。

- 真实登录并列目录、下载 / 修改 / 上传配置；错误密码与访客登录失败。
- 共享目录不能读取运行数据或通过符号链接越界。
- 上传无效 JSON 后，通过真实触摸点击重启：页面显示错误、Go PID 和生效配置保持不变。
- 上传有效变更，真实确认重启：Go PID 变化、配置生效、SMB 持续可访问，目录 / 组件可重新打开。
- 恢复测试前配置并再次重启，检查密钥、运行数据未被覆盖；清除临时账号、认证文件与测试文件。
- 断电 / 整机重启后验证运行库自动解压、SMB 自动启动、Go 启动与账号持久化。未执行的项目明确标注，不用进程存活替代功能证据。

结合 [实机闭环测试](board-validation.md) 保存版本、页面截图和副作用证据。Windows / macOS 原生客户端兼容性须分别实测，Linux 客户端成功不能代替两者。


## 资源文件与音频输出

共享的 `assets/` 是资源暂存目录。替换 `assets/reference.wav`，或上传新文件后修改 `apps.picoclaw.tts.prompt_wav` 为 `assets/新文件.wav`；参考 WAV 为 44 字节至 4 MiB。字体路径也可指向 `assets/`。资源、JSON、密钥和 agent 扩展在校验成功后一起应用；校验失败不会替换生效资源，上传过程中修改文件会要求重试。`assets` 禁止链接和特殊文件，隐藏文件会被忽略，单文件最多 16 MiB、目录总计 32 MiB、512 项、深度 12。部署代码会保留已有资源，避免覆盖自定义声音和字体。

全局音量和音频输出保存在 `config.json` 的 `audio` 下：

```json
"audio": {
  "capture_device": "default",
  "playback_device": "default",
  "mixer_card": "Device",
  "volume": 60,
  "output": "speaker",
  "spdif_device": "iec958:CARD=Device,DEV=0",
  "bluetooth_device": ""
}
```

`volume` 为 0～100，`output` 为 `speaker`（内置扬声器）、`spdif` 或 `bluetooth`。`playback_device` 是内置扬声器的 ALSA 设备，`spdif_device` 是数字输出设备；蓝牙使用 `bluetooth_device` 指定的 MAC 地址。选择蓝牙前先在蓝牙页配对耳机或音箱，然后在系统页选择设备并点击蓝牙。页面选择与音量操作自动保存，下一次播放使用所选输出；从其他应用进入设置时，原应用的前台播放已停止。通过 SMB 编辑的配置仍需点击应用并重启。

界面保存只合并发生变化的音频字段，同时更新生效 JSON 和 SMB 待应用 JSON，保留其他待应用内容。若编辑器正在保存，请等写入完成再操作界面。启动时自动将旧 `data_dir/volume.json` 迁入缺少音量字段的配置，成功后删除旧文件；已存在的 `audio.volume` 优先。音量调整后持久化失败会在系统页显示错误，应用配置前会再次尝试保存。

内置扬声器沿用 CM108B 的增益上限，播放时启用 GPIO12 功放；S/PDIF 使用 GPIO13 使能。蓝牙和 S/PDIF 使用软件音量及静音，ALSA plug 层自动转换语音的单声道 / 24 kHz 格式。部署脚本安装 `/etc/init.d/S45hachimodock-audio`，使用板端已有 BlueALSA，以 `a2dp-source` 注册耳机播放能力；需要 BlueZ、BlueALSA 及 ALSA 插件。更换板型时重新核对 GPIO、ALSA 设备和音频服务。

蓝牙扫描在初始 15 秒后完成首轮读取，发现会话在设置页内继续持有，保证未配对设备对象可用于后续配对；离开设置页取消扫描并等待子进程回收。最近一次结果仍会保留，周期刷新只更新当前设备和连接状态；重新扫描替换未配对的旧结果，关蓝牙清空列表，移除设备会同步清除缓存。优先显示 Alias / Name，解码转义的 UTF-8 名称；无名称的广播显示“未命名设备”，地址保留在详情中。按连接状态、音频类型、配对状态排序，最多显示 24 项；“上次发现”表示 BlueZ 当前已不再枚举该设备。扫描无法保证远端一定广播名称，耳机需要处于配对模式。

名称与类型的解析依据 [BlueZ Device API](https://bluez.readthedocs.io/en/latest/device-api/)，发现行为见 [Adapter API](https://bluez.readthedocs.io/en/latest/adapter-api/)；音频输出接入方式见 [BlueALSA](https://github.com/arkq/bluez-alsa)。真实蓝牙耳机出声、S/PDIF 接收器与 Windows / macOS 共享客户端仍需在对应设备上验收。

本次本地验证覆盖全量 Go 测试、`go vet`、相关包竞态检查、LVGL 原生界面和 native / ARM 构建。板端验证了 S/PDIF 的单声道格式转换，以及 BlueALSA 的启动、D-Bus 就绪、Audio Source 注册、重复启动和停止；测试后恢复原有服务状态。已部署板端并验证系统页的输出切换、音量调整和 JSON 保存；实机蓝牙扫描持续枚举设备，返回首页后停止发现并回收扫描进程。通过系统页完成一次包含 assets 的配置应用重启，配置代次和 Go PID 均更新，SMB 持续运行；音量及其他配置保持不变。实际耳机和数字接收器的声音仍需试听验收。
