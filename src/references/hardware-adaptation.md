# 硬件适配基线

本页说明应用应如何使用当前硬件；固件制作细节仍以 [板级说明](../../board/lyra-zero-w/README.md) 为准。以下“已验证”仅覆盖当前板型与既有接线，不自动适用于其他板卡。

## 系统与构建基线

| 项目 | 当前基线 |
|---|---|
| 核心板 / SoC | Luckfox Lyra Zero W / RK3506B，ARMv7 |
| 系统 | SPI NAND、Buildroot、Linux 6.1.99；已运行 ARM hard-float glibc 应用 |
| SDK | `Luckfox_Lyra_SDK_202610`，板级配置 `luckfox_lyra_zero-w_buildroot_spinand_defconfig` |
| 构建 | Docker；Go 1.25.11；Debian Bookworm ARM hard-float 工具链 |
| UI | LVGL v9.3.0，固定提交及补丁由 [`prepare-lvgl.sh`](../tools/prepare-lvgl.sh) 校验和应用 |
| 调试连接 | 使用实际 SSH 地址，执行前确认设备身份；IP 不属于硬件基线 |
| 安装与服务 | `/opt/hachimodock`；`/etc/init.d/S99hachimodock`；日志 `/var/log/hachimodock.log` |

普通应用开发只构建和部署用户态程序。保持既有 DSI、触摸、USB、Wi-Fi、蓝牙及 SSH 适配；不要因新增应用重刷 boot、替换内核或修改设备树。确需板级变更时单独按 `board/` 的流程验证与保留恢复手段。应用部署的“无备份”约定只针对旧应用 `releases/`。

## 设备与复用入口

| 能力 | 已适配设备 / 接口 | 应用复用方式 |
|---|---|---|
| 显示 | WLK2802MIPI / ST7701S；`/dev/fb0` | `lvgl.Page`、`ui_native.h`，由公共 UI 初始化显示 |
| 触摸 | CST826，I²C2 `0x38`，Linux evdev | 公共 `bridge.c` 自动查找具有 ABS_MT 坐标的事件设备 |
| 按键 | `/dev/gpiochip0`，gpio0 offset 2～5 | `platform/controls` 的 `Event`，不要重复申请同一 GPIO |
| 摇杆 | `/dev/i2c-2`，ADS1015 `0x48`，AIN0/1 | `platform/controls` 的方向与按压事件 |
| 电池 | `/dev/i2c-2`，INA226 `0x40` | `platform/battery`；公共 `status.Monitor` 已定时采集 |
| 声卡 | CM108B，USB `0d8c:0012`，ALSA `Device` | `platform/audio`，使用注入的全局音量与播放服务 |
| 功放 | PAM8403，GPIO0_B4 / GPIO12，高电平开启 | 播放服务调用板端 `hachimodock-speaker-amp` |
| Wi-Fi | `wlan0`、wpa_supplicant 控制套接字 | `platform/connectivity`，复用系统连接与 DHCP |
| 蓝牙 | 系统 BlueZ | `platform/connectivity`；不另建蓝牙守护进程 |
| 时间 | 无可靠 RTC；SNTP、内嵌时区数据 | `platform/clock`、`status.Monitor`；默认 `Asia/Shanghai` |

`core.Manifest.Capabilities` 当前只有网络、麦克风、扬声器等服务需求声明，不是权限沙箱。显示、按键、电池等资源不要为登记能力而随意扩展公共契约。

## 显示、触摸与字体

- 原生面板为 **480×640、RGB888、单 Lane DSI**。应用经 LVGL 旋转 270°后使用 **640×480** 逻辑坐标；页面只按逻辑坐标布局，不再自行旋转输入。
- 固件保留 `simple-panel-dsi` 及已验证的 [wlk2802.dtsi](../../board/lyra-zero-w/wlk2802.dtsi)。不要套用 `hardware/Pinout.md` 中历史 SPI 屏的引脚配置。
- 当前触摸的原生坐标与逻辑坐标关系可写为 `raw_x = 479 - logical_y`、`raw_y = logical_x`，用于本板的 evdev 注入测试。更换分辨率、旋转或触摸配置后需重新标定。
- 触摸驱动复用 `focaltech,ft6236` 兼容接口并采用轮询；这不等于实物芯片是 FocalTech。不要硬编码 `/dev/input/event0`，先核对 `/proc/bus/input/devices`。
- 处理 ABS_MT slot、tracking ID、按下及释放；非零 tracking ID 也是有效触点。触摸缺失时目录仍可显示，设备恢复后目前需重启应用重新打开。
- 使用公共 18/22/30 像素字体和符号回退。文泉驿微米黑负责中文，邻目录 `NotoEmoji.ttf` 补充单色 Emoji，Montserrat 提供键盘私用图标；不要把缺字方块误判为网络编码错误。
- 中文、Emoji 的原始字节由各字号共享，当前约占 7 MiB 内存。不要在页面每次渲染时重新加载字体。键盘目前只有英文和符号输入，没有中文输入法。
- 截图是帧缓冲的采集结果，仍需观察实屏的方向、颜色和触控；截图正常不能排除面板、接线或触摸硬件问题。

## 按键、摇杆和资源所有权

| gpio0 offset | 信号 | 电气与当前约定 |
|---:|---|---|
| 2 | SW1，GPIO0_A2 | 输入上拉，按下为低；PicoClaw 使用按下与释放实现按住录音 |
| 3 | SW2，GPIO0_A3 | 输入上拉，按下为低；目录打开所选应用，应用可映射主操作 |
| 4 | SW3，GPIO0_A4 | 输入上拉，按下为低；公共返回 / 屏保入口 |
| 5 | 摇杆按键，GPIO0_A5 | 输入上拉，按下为低；目前公共框架未绑定业务动作 |
| 12 | SPK_EN，GPIO0_B4 | 功放输出控制，不属于按键输入组 |

按键约每 5 ms 读取，稳定 20 ms 后触发；摇杆启动时居中校准约半秒，方向死区 20%、回差 5%，长拨 400 ms 后每 150 ms 重复。选项 `swap_xy` 先于 `invert_x/invert_y`。

当前底板原始 X+ 对应物理向右、Y+ 对应物理向上，配置 `ui.controls.invert_y: true`，其余轴选项为 false。输入层统一归一化后，物理向上产生 `controls.Up`、向下产生 `controls.Down`；组件中心按方向移动，音乐页按 Up 增加音量、Down 减少音量。应用不要再自行补偿底板轴方向。修改接线、安装方向或换板后重新实测四方向；省略 `invert_y` 时仍按 Go 布尔零值 false 解析。

当前内核的 GPIO 上拉请求不足以完成适配，公共驱动会先核对设备树，再通过 `/dev/mem` 临时修改 `0xff950200` 中 A2～A5 的字段，并在正常退出或 SIGTERM 时恢复。新应用不得复制这段固定地址操作，也不能改动 A0/A1 所在的 I²C 引脚。SIGKILL、断电无法保证执行恢复。

ADS1015 使用 `/tmp/hachimodock-ads1015.lock`，INA226 读取使用 `/dev/i2c-2` 文件上的建议锁；它们不是同一把锁。建议锁不会阻止不配合的脚本。独立运行 `sandbox/keyboard`、`sandbox/battery` 试验前停止应用服务，结束后恢复服务。禁止并行修改同一芯片的转换配置、强制抢占内核已绑定地址或进行广播复位。

电池参数为 10 mΩ 采样电阻、0.25 mA 电流 LSB，采样后恢复原配置与校准；放电为正、充电为负。公共监视器约每 3 秒采样。电量百分比只是单节锂电池电压粗估，未完成电芯标定，不用于充电保护或精确续航预测。IP5306 `0x75` 是原理图中的预留资源，目前没有对应的应用服务实现。

显示估算使用 3.4 / 3.5 / 3.6 / 3.7 / 3.8 / 3.9 / 4.0 / 4.1 / 4.2 V 对应 0 / 5 / 10 / 20 / 40 / 60 / 80 / 90 / 100%，区间线性插值，见 [`battery/level.go`](../internal/platform/battery/level.go)。图标按 ≥20% 绿色、10～19% 橙色、<10% 红色显示；充电电流低于 −5 mA 时显示闪电，读取失败或电压异常时显示未知状态。

## 音频、功放和全局音量

| 链路 | 格式 / 约束 |
|---|---|
| 语音采集 → ASR | PCM16 LE、16 kHz、单声道、无 WAV 头 |
| TTS → 语音播放 | PCM16 LE、24 kHz、单声道、流式读入 |
| 音乐解码 → ALSA | PCM16 LE、48 kHz、双声道，经 FFmpeg 解码 |
| 硬件与转换 | CM108B 原生 44.1/48 kHz；`plughw:CARD=Device,DEV=0` 负责采样率 / 声道转换；当前唯一声卡时 `default` 也已验证 |
| 音量 | 默认 60% → `17,17`（−20 dB）；100% → 已试听上限 `29,29`（−8 dB）；0% 关闭播放开关 |

新应用须注入 `bootstrap` 创建的同一个 `audio.Volume`，经 `audio.ALSA` 或 `audio.StreamPlayer` 使用共享输出锁、功放和音量。不要绕过服务直接启动第二个 aplay，或把 ALSA 的最大原始值 37 当成应用音量上限。

内置扬声器播放前开启 GPIO12 功放，S/PDIF 使用 GPIO13 使能，蓝牙保持两者关闭。完成、失败、取消与离开页面均关闭物理输出、回收 aplay / FFmpeg、关闭网络流。共享播放锁只作用于同一应用进程，独立测试程序仍需与正常播放互斥。板载 FFmpeg 未启用 HTTPS，音乐服务已通过仅绑定回环地址的 Go HTTP 桥解决输入问题，其他音源应复用或明确适配。

[USB 音频文档](../../board/lyra-zero-w/usb-audio.md) 保留了驱动安装与初期混音器测试记录；当前 `platform/audio` 已自动控制功放和音量上限。部分长文本 TTS 曾独立复现流错误或空音频，验收时应区分服务失败和本地声卡失败，不能只凭 HTTP 状态或 aplay 进程判断声音正确。

## 网络、时间及适配边界

Wi-Fi 复用 `/var/run/wpa_supplicant/wlan0`、系统 DHCP 和既有持久化配置。蓝牙当前支持无需输入 PIN 的配对及常用设备管理，没有实现键盘 PIN 交互。蓝牙音频路由通过 BlueALSA A2DP source 接入，服务与输出选择见 [音频配置](configuration-sharing.md#资源文件与音频输出)。代码已包含 [`platform/bluez`](../internal/platform/bluez/) 的 Rockchip 接口兼容处理；历史实测仅确认经典模式的发现、配对和软件侧 A2DP，实际发声及 Wi-Fi 共存仍未验收，模式限制见 [板级无线说明](../../board/lyra-zero-w/wifi.md#经典蓝牙音频与默认-ble-模式)。不由旧记录推断当前设备模式或部署状态。电池 / 无线采集由公共监视器管理，应用页面不要再启动一套轮询。

重启后时间可能回到 1970 年，先确认 SNTP 成功再判断 HTTPS / WSS 证书错误。`DOCK_SYNC_TIME=1` 的重试不应阻塞界面；显示时区由 `ui.timezone` 控制。测试报告同时记录主机和板端时间，不能直接把未对时的板端日志当作准确日期。

换板时至少复核：显示格式和旋转、触摸范围、GPIO 芯片与 offset、I²C 总线及地址、功放极性、ALSA 名称和音量刻度、模块 / 内核匹配、可用工具和服务路径。先在平台层完成适配，再让应用复用接口；不在页面中散布新板型的寄存器地址或 shell 命令。
