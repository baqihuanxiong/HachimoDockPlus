# HachimoDock Go + LVGL

面向 Lyra Zero W / RK3506B 的首版应用核心。当前提供 640×480 横屏应用目录、PicoClaw 对话页、文字输入和按键式语音对话，网易云音乐播放器，以及 Wi-Fi / 蓝牙 / 音频设置页。

## 结构与边界

```text
src/
├── cmd/
│   └── dock/                 依赖组装、启动参数、诊断入口
├── internal/
│   ├── core/                 应用契约、注册表、生命周期、导航和事件分发
│   ├── bootstrap/            具体应用、依赖和页面的唯一装配入口
│   ├── apps/
│   │   ├── picoclaw/         对话状态、语音轮次、取消、会话和应用配置
│   │   ├── music/            网易云短信登录、封面、歌词及播放状态
│   │   └── settings/         无线设置、外观偏好及前台操作
│   ├── preferences/          持久化外观偏好与主题色定义
│   ├── agent/
│   │   └── picoclaw/         嵌入 PicoClaw AgentLoop 的适配器
│   ├── speech/               FunASR WebSocket、CosyVoice HTTP 流
│   ├── platform/
│   │   ├── audio/            ALSA 采集、流式解码播放及全局音量
│   │   ├── controls/         GPIO 按键、ADS1015 摇杆与导航事件
│   │   ├── clock/            无 RTC 设备的 SNTP 对时
│   │   ├── connectivity/     wpa_supplicant 控制套接字与 BlueZ 适配
│   │   ├── battery/          INA226 电压、电流采样及配置恢复
│   │   └── status/           首页后台状态采集与时区显示
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

`bootstrap.sh` 准备文泉驿微米黑中文字体、内置应用图标和 CosyVoice 官方示例参考音频，许可及来源保存在 `.local/assets/`。默认使用 `instruct2`，需要参考 WAV 和语气指令；若切换为 `zero_shot`，还必须提供参考音频的准确转写 `prompt_text`。

生成的 `build/arm/dock` 约 23 MiB，动态依赖 ARM glibc；LVGL 链入程序。当前官方 Buildroot glibc 2.38 已实机运行，musl 系统需要更换交叉工具链。

## 运行和部署

```sh
# 主机生成两张真实 LVGL 预览，无需显示设备
tools/dev.sh build/native/dock -config .local/config.json -preview build/catalog.png
tools/dev.sh build/native/dock -config .local/config.json -preview build/picoclaw.png -page picoclaw

# 文本交互诊断；/voice、/stop、/new 为 PicoClaw 应用命令
tools/dev.sh build/native/dock -config .local/config.json -ui terminal -page picoclaw

# 部署 ARM 程序、字体、配置和密钥
tools/deploy.sh root@192.168.4.202 --no-backup
```

部署位置为 `/opt/hachimodock`。程序按内容哈希放入 `releases/`，`current` 指向当前版本。后续部署保留板端配置、密钥和数据；如果开机服务已经运行，会停止服务、切换版本后重新启动。本地部署包使用临时目录，结束后删除；加 `--no-backup` 时切换版本后删除板端所有旧应用版本，仅保留当前版本。省略该参数则保留板端旧版本。空间不足时会在停止服务前退出，可先清理未使用的旧 `releases/`。首次部署后安装开机服务：

```sh
tools/install-autostart.sh root@192.168.4.202
```

安装脚本部署 `/etc/init.d/S99hachimodock` 和 `/opt/hachimodock/dock-run`，立即启动应用，并保留已有 `/etc/default/hachimodock`。界面无需等到联网即可进入目录，主进程退出后等待三秒重启。服务支持 `start|stop|restart|status`；日志位于 `/var/log/hachimodock.log`（上次运行 `.log.1`，随重启清空）。更新开机脚本后重新运行安装命令。

手动运行前先执行 `/etc/init.d/S99hachimodock stop`，避免多个进程同时绘制屏幕：

```sh
ssh root@192.168.4.202
/opt/hachimodock/current/dock -sync-time
/opt/hachimodock/current/dock -config /opt/hachimodock/config.json
```

开发板缺少有效 RTC 时，重启后可能回到 1970 年，必须先对时，否则 HTTPS/WSS 证书校验会失败。当前板端通过 `/etc/default/hachimodock` 中的 `DOCK_SYNC_TIME=1` 启用后台 SNTP 重试，最多尝试 120 次，离线启动也不会阻塞界面。没有现成 NTP 服务时才启用；已有 NTP 服务或旧 `S49dock-time` 时保持为 `0`，避免重复对时。对时日志为 `/var/log/hachimodock-time.log`；`-ntp-servers host:123,...` 可指定局域网时间源。对时需要 root 或 `CAP_SYS_TIME`。

显示使用 `/dev/fb0`，原生面板保持 480×640，由 LVGL 软件旋转 270°得到逻辑 640×480；不修改 DSI 原生分辨率。中文字体以 22/30 像素绘制。触摸会检查指定设备后扫描 `/dev/input/event*`，按 ABS_MT 坐标识别触摸屏，避免误选电源键；多点协议中的任意有效 tracking ID 均可触发按下。旋转后的坐标由 LVGL 处理。

物理显示和 CST826 的系统配置见 [Lyra Zero W 板级说明](../board/lyra-zero-w/README.md)。当前固件显示 Linux 控制台；启动应用后由 LVGL 绘制界面，RGB888 颜色、文字及横屏触控此前已通过实机验证。没有内核输入设备时应用会提示“未检测到触摸设备”，触摸恢复后需重启应用重新打开输入设备。屏幕键盘支持英文与符号输入，尚未实现中文输入法；中文字体回退到 LVGL 内置 Montserrat 符号字体绘制功能键，避免出现方块。

Lyra Zero W 的 CM108B 已注册为 ALSA 声卡 `Device`，板端录放音及扬声器到麦克风的声学测试通过。`audio.capture_device` / `playback_device` 可使用 `plughw:CARD=Device,DEV=0`，支持应用所需的 16 kHz 单声道采集和 24 kHz 单声道播放；当前仅有这一张声卡时 `default` 也已验证可用。底板扬声器播放前需执行 `hachimodock-speaker-amp on`，使用结束后执行 `hachimodock-speaker-amp off`；音乐和语音播放已自动开启功放，播放结束、取消或失败后关闭。驱动与功放工具安装方法见 [板级音频说明](../board/lyra-zero-w/usb-audio.md)。

## 组件中心与主题色

应用目录采用每页两列、两行的四卡布局，按注册顺序排列；应用超过四个时自动新增页面。左右滑动整页吸附，不显示滑动条，底部圆点表示总页数与当前页，也可点击圆点跳页。拖动卡片不会打开应用，点击卡片的图标、标题或空白处均可打开。当前内置 PicoClaw、设置和网易云音乐，共一页；不添加用于占位的假应用。

PicoClaw 卡片使用随包附带的官方透明 PNG，网易云卡片使用官方 favicon 转换的透明 PNG，设置卡片使用居中绘制的内置齿轮符号，离线即可显示。图片等比缩放并保留原色；新增应用可通过 `Manifest.IconImage` 提供 PNG，缺失时回退到 `Icon`。来源及许可见 [应用图标资源](assets/app-icons/README.md)。

卡片右上角的“运行中”来自实际运行状态。可选 `core.BackgroundReporter` 接口的 `BackgroundRunning()` 用于上报独立后台工作的状态，不能执行阻塞 I/O。现有 PicoClaw 和设置离开时取消前台任务，缓存的会话实例不会显示为后台运行；本次没有改变它们的生命周期或引入后台调度。

“设置 → 外观”提供青绿、蓝色、紫色、琥珀、玫红、青色、橙色、绿色和红色九种主题色（三列三行），选择后立即应用到目录图标底色、分页圆点、按钮、活动选项卡和强调文字。白天/黑夜模式独立于主题色，电池等状态颜色的含义保持固定。“外观”页顶部可手动切换白天模式（浅色背景、深色文字）与黑夜模式（深色背景、浅色文字）。首页、设置、PicoClaw、输入框、键盘及状态栏随模式一起更新；切换主题色会保留当前模式，切换模式也保留主题色。旧偏好文件缺少 `mode` 时按黑夜模式加载。

主题色、模式和屏保选择通过临时文件、`fsync` 和原子重命名写入 `data_dir/preferences.json`（权限 `600`），重启后恢复，不修改模型配置、网络密码或会话文件。

## 按键、摇杆与屏保

正常 LVGL 运行时自动读取底板物理输入，触屏继续可用；预览和终端模式不申请按键或摇杆设备。

| 输入 | 行为 |
|---|---|
| SW3 | 组件中心进入屏保，屏保中返回组件中心；应用内返回，弹窗或软键盘打开时先关闭 |
| SW2 | 在组件中心打开当前选中的应用；网易云播放器内播放 / 暂停；其他应用与屏保中暂不绑定 |
| SW1 | 保留，尚未绑定录音动作 |
| 摇杆四方向 | 组件中心移动选中框，左右跨页时保持所在行；音乐页左右切歌、上下调整全局音量（每次 5%） |
| 摇杆按下 | 保留，尚未绑定动作 |

选中框默认位于第一页第一个应用，触屏翻页后移到该页首个应用；触摸卡片或按 SW2 可打开所选应用，摇杆仅选择。方向按 X+ 向右、Y+ 向下，不交换轴、不反转。按钮稳定 20 ms 后触发，SW2/SW3 只响应按下；摇杆方向死区为 20%、回差 5%，长拨 400 ms 后每 150 ms 连续移动，斜推取幅度较大的一轴。

“设置 → 外观 → 屏保模式”可切换时钟屏保和纯黑屏保，默认时钟，选择保存到 `data_dir/preferences.json`，旧偏好文件自动使用时钟屏保。时钟每秒更新，使用 `ui.timezone`（默认上海）；系统尚未对时时显示“等待对时”。纯黑屏保只绘制黑色，不关闭背光。触摸屏保或按 SW3 也可回组件中心；摇杆不会唤醒屏保。当前在组件中心按 SW3 手动进入，没有闲置自动进入功能；应用内先按 SW3 返回组件中心，再按进入屏保。

GPIO 使用 `/dev/gpiochip0` 的 offset 2～5，摇杆使用 `/dev/i2c-2` 的 ADS1015 `0x48`；后台分别采集，两者的读取失败互不禁用，触屏仍可使用。启动时保持摇杆居中约半秒用于校准。当前内核需要 `/dev/mem` 临时设置 RK3506 上拉，程序先验证设备树，再仅修改 GPIO0_A2～A5，正常退出或 SIGTERM 时恢复上拉和 ADC 配置；硬件细节见 [按键试验](../sandbox/keyboard/README.md)。程序占用 GPIO 期间不能运行该试验脚本，也不要同时使用其他 ADS1015 读取程序。

可在配置 `ui` 下加入以下可选项；省略时均为 `false`，保留当前实物方向。开发机需要禁用物理输入时设置 `disabled: true`：

```json
"controls": {
  "disabled": false,
  "swap_xy": false,
  "invert_x": false,
  "invert_y": false
}
```

交换轴先于反转方向执行。启动失败会在日志中说明对应设备错误；恢复硬件后需重启程序重新初始化输入。

## 首页顶部栏

顶部栏仅一行：左侧“组件中心”，中间以 `11-14 Sat 20:30` 显示月日、三字母星期与时间，右侧为 Wi-Fi、蓝牙设备和电池图标，不显示电压、百分比或无线文字。`ui.timezone` 默认 `Asia/Shanghai`，时区数据嵌入二进制，不依赖板端 `/etc/localtime`；系统仍为 1970 年时显示“等待对时”，SNTP 成功后自动更新。

Wi-Fi 未连接时显示划线图标，连接后通过 `SIGNAL_POLL` 的 RSSI 显示四档亮度弧线，分界为 −75 / −65 / −55 dBm；图标反映本机关联状态，不探测互联网可达性。蓝牙开启但没有设备连接时隐藏图标，连接后根据 BlueZ `Icon` / `Class` 显示耳机、音箱、键盘、鼠标、手柄、手机、电脑、手表或相机图标，未知类型回退为蓝牙符号。多设备连接时优先显示音频设备，其余按地址排序选一个，避免图标跳变。

电池读取 `/dev/i2c-2@0x40` 的 INA226，沿用 [电池测试](../sandbox/battery/README.md) 的 10 mΩ 采样电阻、0.25 mA 电流量化、触发转换和电池侧电压补偿。每三秒采一组，采样后恢复原配置与校准，负电流低于 −5 mA 时在电池内显示充电闪电。

电池图标的填充和颜色暂按普通 4.2 V 单节锂电池电压粗估：使用 3.4 V / 0、3.5 V / 5、3.6 V / 10、3.7 V / 20、3.8 V / 40、3.9 V / 60、4.0 V / 80、4.1 V / 90、4.2 V / 100 的近似曲线并线性插值；这是一条显示用默认曲线，未对当前电芯标定。估算值 ≥20 绿色、10～19 橙色、<10 红色，数字不显示。负载、充电和温度均会影响电压估算，不能将颜色阈值视为精确剩余容量或用于电池保护；参见 [TI 的电压估算说明](https://www.ti.com/document-viewer/lit/html/SSZT786/GUID-5DD8C3D9-BCBE-467B-90E5-FAA672AAC15D)。读取失败、设备忙、超出合理电压范围或设备缺失时显示灰色未知电池图标，后续采样成功会恢复。

电池和无线状态分别在后台采集，无线每五秒只读查询，所有 LVGL 调用仍在 UI 线程。切换到应用后后台状态继续更新，返回首页显示最新状态；退出程序会取消并等待采集结束。电池读取配合试验脚本的建议锁，独占试验可先停止 `/etc/init.d/S99hachimodock`，结束后重新启动。

## 网易云音乐与音频设置

在组件中心打开“网易云音乐”，未登录时直接输入中国大陆手机号（+86），获取短信验证码后登录。短信发送提供 60 秒重发间隔，手机号和验证码不保存；已删除扫码登录。Go 直接访问网易云，无需 Node.js 代理服务或额外配置。

登录后可选择歌单与歌曲，歌单名在两种主题下都保持可见、左侧统一留白。歌曲列表每页 20 首，支持顺序循环、上一首 / 下一首、暂停 / 继续。左侧唱盘显示歌曲封面，点击切换到随播放位置更新的歌词，再点歌词回到唱盘；缺少封面或歌词时显示默认图标或“暂无歌词”，不影响播放。

主界面只保留音量图标，点击弹出音量条，点击弹层外部或按 SW3 关闭；摇杆调整音量不依赖弹层。已按当前底板实际 Y 轴方向修正为物理向上增加、向下减少。进度条可拖动，释放后实际跳转，并保留暂停状态；试听片段不开放进度拖动。SW3 在键盘、音量弹层或歌单列表中先关闭当前浮层，再按返回组件中心并停止播放。
“设置 → 音频”与播放器共用全局音量，音乐和 PicoClaw 语音播放同步生效。默认 60%（CM108B PCM `17,17`，−20 dB），0% 关闭播放开关静音，100% 限制为已试听的 `29,29`（−8 dB），不使用芯片的最高增益。音量保存到 `data_dir/volume.json`，登录会话保存到 `data_dir/apps/music/session.json`，均为权限 `600` 的文件；退出账号会删除本地登录凭据。

播放器依赖板载 `ffmpeg`、`aplay`、`amixer` 和已安装的功放控制工具。`audio.mixer_card` 可选，默认 `Device`；`audio.playback_device` 继续决定 PCM 输出设备。由于板载 FFmpeg 未编译 HTTPS，由 Go 通过临时回环 HTTP 桥接 HTTPS 音频并转发 Range 请求，让 FFmpeg 可按位置跳转；解码为 48 kHz / PCM16 / 双声道后通过管道交给 ALSA，不会整首下载到内存或 NAND。会员、试听和版权限制依网易云账号实际权限提示，不提供解锁。

接口、验证方法与限制见 [音乐应用说明](docs/music.md)。

## 无线设置

在应用目录进入“设置”，顶部切换 Wi-Fi / 蓝牙。进入时读取状态，前台每五秒刷新；扫描与连接期间显示处理中，返回会取消任务，再次进入重新读取。

- Wi-Fi：显示当前 SSID、IP、已保存网络及扫描信号。支持扫描、手动添加 WPA/WPA2 Personal、开放热点连接、已保存网络重连、断开和忘记。密码默认隐藏；连接并取得 IP 后才调用 `SAVE_CONFIG`，失败或取消时尝试恢复原网络。忘记会立即删除对应保存项，修改密码可先忘记再连接。断开仅影响本次运行，重启仍会自动连接保存网络。
- 蓝牙：支持电源开关、八秒扫描、配对、连接、断开和移除配对。当前使用 `NoInputNoOutput` 代理，只支持无需输入 PIN / 数字确认的配对；键盘 PIN、手机数字确认未实现。连接能力由系统 BlueZ 提供的 profile 决定，本页不安装蓝牙音频服务，也不切换 PicoClaw 的 ALSA 声卡。配对记录由 BlueZ 保存，电源开关遵循系统 BlueZ 重启策略。

Wi-Fi 通过 `/var/run/wpa_supplicant/wlan0` 直接通信，密码不放入命令行、日志或应用快照；输入 SSID 使用十六进制编码，口令派生为 PSK。Wi-Fi 保存依赖 `update_config=1` 和可写配置，板端 `/var/run/wpa_supplicant/wpa_supplicant.conf` 已链接到 `/etc/wpa_supplicant.conf`，配置保持 `600` 权限。沿用系统 DHCP 和无线初始化流程，不另起 supplicant 或 DHCP 服务。纯 WPA3、企业 EAP 和 WEP 热点会显示为不支持。

开发机无这些设备或服务时，设置页显示相应不可用信息，不影响目录和 PicoClaw。终端诊断可使用 `-ui terminal -page settings`，然后输入 `/scan`、`/bt-scan` 或 `/event wifi.connect 0` 等应用动作。协议参考：[wpa_supplicant 控制接口](https://w1.fi/wpa_supplicant/wpa_supplicant-devel.pdf)、[BlueZ bluetoothctl](https://kernel.googlesource.com/pub/scm/bluetooth/bluez.git/+/refs/tags/5.83/client/bluetoothctl.rst)。

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

设置测试覆盖热点解析与中文 SSID、输入校验、密码不进入控制命令明文、成功后保存、失败回退、套接字取消、页面重入、快照副本和错误提示。原生 LVGL 测试覆盖设置页反复挂载、Wi-Fi 密码弹窗、JSON 转义、列表刷新时输入保持和蓝牙事件。

架构测试覆盖注册冲突、按需初始化、两个应用之间的切换和取消、旧页面事件拒绝、启动失败回到目录、重复关闭、应用重入及旧配置的数据路径兼容。原生 LVGL 测试另外检查多个目录项生成、符号字体回退和页面反复挂载/卸载。

运行日志不打印密钥，服务错误不回显完整响应。工作区和会话可能包含对话原文，`.local/`、`build/`、`.cache/` 及 `references/secret.md` 都不应提交。此首版不提供后台鉴权接口、自动更新和应用热加载。
