# HachimoDock Go + LVGL

面向 Lyra Zero W / RK3506B 的首版应用核心。当前提供 640×480 横屏应用目录、PicoClaw 对话页、文字输入和按键式语音对话。

## 结构与边界

```text
src/
├── cmd/
│   └── dock/                 依赖组装、启动参数、诊断入口
├── internal/
│   ├── core/                 应用契约、注册表、生命周期、导航和事件分发
│   ├── bootstrap/            具体应用、依赖和页面的唯一装配入口
│   ├── apps/
│   │   └── picoclaw/         对话状态、语音轮次、取消、会话和应用配置
│   ├── agent/
│   │   └── picoclaw/         嵌入 PicoClaw AgentLoop 的适配器
│   ├── speech/               FunASR WebSocket、CosyVoice HTTP 流
│   ├── platform/
│   │   ├── audio/            ALSA 采集与播放
│   │   └── clock/            无 RTC 设备的 SNTP 对时
│   └── ui/
│       ├── lvgl/             公共显示、动态目录、触摸及独立 page_* 页面
│       └── terminal/         无屏幕诊断入口
├── docs/
│   └── app-development.md    新增应用的规范、接口和验收流程
├── tests/                    原生 LVGL 输入、目录和页面生命周期验证
└── tools/                    可复现构建、配置导入和部署
```

Go 负责业务和网络，所有 LVGL 调用由一个锁定的 OS 线程执行。应用注册表自动生成可滚动目录；首次打开应用时才调用工厂初始化依赖。每次进入获得独立的 `context`，离开会取消前台任务并调用 `Stop`，进程退出调用 `Close` 等待清理。应用实例保留会话状态，页面组件则在进入、离开时创建和销毁。

后台工作通过带版本号的状态快照更新界面，C 侧事件队列把点击交给 Go。事件携带应用 ID，离开页面的事件不会转发到另一个应用；PicoClaw 通过轮次编号忽略已取消请求的迟到结果。公共核心与前端循环不依赖 PicoClaw，依赖方向由自动测试检查。

语音链路为：ALSA PCM16/16kHz 单声道 → FunASR 实时识别 → 已确认的最终文本 → 嵌入式 PicoClaw → CosyVoice → PCM16/24kHz 单声道播放。点击“开始说话”录音，再次点击结束并发送；最长录音 30 秒，每轮上限 3 分钟，处理中可以停止。当前模型回复完成后才进行语音合成；未实现全双工对话。

[PicoClaw](https://github.com/sipeed/picoclaw/tree/v0.3.1) 固定为 v0.3.1，通过 Go 包直接创建 AgentLoop，不需要另起 PicoClaw 服务。当前对话配置关闭全部工具、技能和 MCP；启动时验证实际工具列表为空。工作区、会话和模型记忆由 PicoClaw 存入 `data_dir`，新对话使用独立会话键。升级上游时只需集中复核适配器及其会话测试。

新增应用按 [应用开发规范](docs/app-development.md) 实现 `core.Application`、应用配置及 `lvgl.Page`，在 `bootstrap/apps.go` 装配注册即可。目录生成、公共导航、事件循环和构建脚本不需要修改。未来局域网代理可实现 PicoClaw 应用的 `Agent` 接口，或作为独立应用注册。当前采用编译期注册、单进程运行，不提供二进制插件热加载。

## 构建

主机需要 Docker、Git、SSH 和网络。构建容器使用 Go 1.25.11、Debian Bookworm ARM hard-float 交叉工具链，LVGL 固定为 v9.3.0 的指定提交。无需在开发板上安装编译器。以下命令从 `src/` 执行：

```sh
tools/dev.sh tools/bootstrap.sh
python3 tools/import_references.py
tools/dev.sh tools/build.sh native
tools/dev.sh tools/build.sh arm
```

配置导入会读取本地 `references/secret.md`，生成被 Git 忽略的 `.local/config.json` 和三个权限为 600 的密钥文件，不打印密钥。该命令会覆盖现有本机生成配置，手工修改过配置后不要重复导入。没有该参考文件时，将 `config.example.json` 复制为自己的配置，填写地址、模型和密钥文件位置即可；所有相对路径都相对于配置文件所在目录。

应用配置放在 `apps.<id>` 下，打开应用时才验证服务地址和读取密钥，目录本身不需要模型密钥。新配置的数据默认放在 `data_dir/apps/<id>/`；旧版顶层 `llm/asr/tts` 配置仍可直接使用，程序在内存中转换并继续使用原数据目录，不改写板端配置或搬动旧会话。应用的 `data_dir` 可显式指定已有数据位置。

`bootstrap.sh` 准备文泉驿微米黑中文字体和 CosyVoice 官方示例参考音频，许可及来源保存在 `.local/assets/`。默认使用 `instruct2`，需要参考 WAV 和语气指令；若切换为 `zero_shot`，还必须提供参考音频的准确转写 `prompt_text`。

生成的 `build/arm/dock` 约 23 MiB，动态依赖 ARM glibc；LVGL 链入程序。当前官方 Buildroot glibc 2.38 已实机运行，musl 系统需要更换交叉工具链。

## 运行和部署

```sh
# 主机生成两张真实 LVGL 预览，无需显示设备
tools/dev.sh build/native/dock -config .local/config.json -preview build/catalog.png
tools/dev.sh build/native/dock -config .local/config.json -preview build/picoclaw.png -page picoclaw

# 文本交互诊断；/voice、/stop、/new 为 PicoClaw 应用命令
tools/dev.sh build/native/dock -config .local/config.json -ui terminal -page picoclaw

# 部署 ARM 程序、字体、配置和密钥
tools/deploy.sh root@192.168.5.156
```

部署位置为 `/opt/hachimodock`。程序按内容哈希放入 `releases/`，`current` 指向当前版本；后续部署保留板端配置、密钥和数据。部署脚本不启动程序、不修改启动镜像，也不安装开机服务。更新前先结束正在使用 framebuffer 的旧进程，再启动新版本：

```sh
ssh root@192.168.5.156
/opt/hachimodock/current/dock -sync-time
/opt/hachimodock/current/dock -config /opt/hachimodock/config.json
```

开发板缺少有效 RTC 时，重启后可能回到 1970 年，必须先对时，否则 HTTPS/WSS 证书校验会失败。`tools/S49dock-time` 是可选的 Buildroot 开机对时脚本，仅在系统没有现成 NTP 服务时安装；`-ntp-servers host:123,...` 可指定局域网时间源。对时需要 root 或 `CAP_SYS_TIME`。

显示使用 `/dev/fb0`，原生面板保持 480×640，由 LVGL 软件旋转 270°得到逻辑 640×480；不修改 DSI 原生分辨率。中文字体以 22/30 像素绘制。触摸会检查指定设备后扫描 `/dev/input/event*`，按 ABS_MT 坐标识别触摸屏，避免误选电源键；多点协议中的任意有效 tracking ID 均可触发按下。旋转后的坐标由 LVGL 处理。

物理显示和 CST826 的系统配置见 [屏幕适配记录](../board/lyra-zero-w-wlk2802/README.md)。RGB888 输出已由用户确认颜色和文字正常。触摸曾发生 I²C 超时，断电重新插牢排线后恢复；用户已确认横屏坐标、页面切换、输入发送及真实模型回复正常。没有内核输入设备时应用会提示“未检测到触摸设备”，触摸恢复后需重启应用重新打开输入设备。屏幕键盘支持英文与符号输入，尚未实现中文输入法；中文字体回退到 LVGL 内置 Montserrat 符号字体绘制功能键，避免出现方块。

开发板当前 `/proc/asound/cards` 为 `no soundcards`，因此录音按钮禁用，仍可通过文字与模型对话。真实麦克风和扬声器尚未验收。接入音频设备后配置 `audio.capture_device` / `playback_device`（如 `plughw:1,0`），并确认 `arecord` / `aplay` 可用；使用 `plughw` 可由 ALSA 转换采样率。

## 验证

```sh
tools/dev.sh sh -c 'go test -race -p 2 -tags stdjson,goolm ./... && go vet -tags stdjson,goolm ./...'
tools/dev.sh sh tools/check-ui.sh
tools/dev.sh build/native/dock -config .local/config.json -ask '用一句话介绍自己'
tools/dev.sh build/native/dock -config .local/config.json -tts '你好' -pcm-output build/hello-24k.pcm
# 输入必须是无 WAV 文件头的 PCM16 LE、单声道、16kHz
tools/dev.sh build/native/dock -config .local/config.json -asr-pcm build/input-16k.pcm
```

自动测试覆盖识别最终确认、累计结果替换、断流与取消、TTS multipart 与 PCM 对齐、取消后旧结果失效、文本降级、密钥权限，以及 PicoClaw 真实 AgentLoop 的上下文保留和会话隔离。原生 LVGL 测试注入多点 slot 7、tracking ID 19 的 Linux 输入事件，验证旋转后的坐标和目录按钮点击。模型、TTS、ASR 已使用参考服务做实际请求验证；这不等于板端麦克风到扬声器的完整硬件验收。

架构测试覆盖注册冲突、按需初始化、两个应用之间的切换和取消、旧页面事件拒绝、启动失败回到目录、重复关闭、应用重入及旧配置的数据路径兼容。原生 LVGL 测试另外检查多个目录项生成、符号字体回退和页面反复挂载/卸载。

运行日志不打印密钥，服务错误不回显完整响应。工作区和会话可能包含对话原文，`.local/`、`build/`、`.cache/` 及 `references/secret.md` 都不应提交。此首版不提供后台鉴权接口、自动更新和应用热加载。
