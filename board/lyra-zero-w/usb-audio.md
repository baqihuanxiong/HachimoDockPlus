# CM108B USB Audio

CM108B（`0d8c:0012`）使用内核标准 `snd-usb-audio` 驱动，无需独立设备树声卡节点。当前方案针对 SDK 202610 的 Linux 6.1.99，通过加载模块扩展现有固件，保留 RGB888 屏幕、触摸及网络配置。

## 构建

主机使用 Docker。SDK 内核源码及配套 GCC 10.3.1 工具链需位于 `sdk/luckyfox-lyra/source/`。从项目根目录执行：

本次内核源码提交为 `1e631165ecec0ed841de793305423acd61a2bbdd`。模块 vermagic 为 `6.1.99 SMP preempt mod_unload ARMv7 thumb2 p2v8`，与板上原模块一致；同配置重编译的 `btusb`、`btintel` 代码、数据及模块结构段均与板端版本一致。

```sh
docker build -t hachimodock-lyra:dev \
  -f board/lyra-zero-w/Dockerfile board/lyra-zero-w
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD:/work" hachimodock-lyra:dev \
  bash board/lyra-zero-w/build-usb-audio.sh
```

[usb-audio.config](usb-audio.config) 开启标准 USB Audio 模块；Kconfig 自动选择 `SND_HWDEP=m`、`SND_RAWMIDI=m` 和驱动内部的 Media Controller 支持。构建脚本核对相对 SDK 屏幕配置只增加这四项，内核版本必须为 `6.1.99`。编译 `vmlinux` 用来生成模块符号表，不部署该内核。

输出位于 `sdk/luckyfox-lyra/artifacts/cm108b/`，包含配置、符号表及 `usb-audio-modules.tar.gz`。包内四个模块为：

```text
lib/modules/6.1.99/kernel/sound/core/snd-hwdep.ko
lib/modules/6.1.99/kernel/sound/core/snd-rawmidi.ko
lib/modules/6.1.99/kernel/sound/usb/snd-usbmidi-lib.ko
lib/modules/6.1.99/kernel/sound/usb/snd-usb-audio.ko
```

## 加载与部署

先把包解压到板端临时目录并运行 `sha256sum -c SHA256SUMS`，核对模块 vermagic 与板端已有模块一致，再按上述顺序 `insmod`，检查 `dmesg`、`/proc/asound/cards`、`aplay -l` 和 `arecord -l`。不使用强制加载参数。

通过临时加载验证后，将四个模块安装到对应的 `/lib/modules/6.1.99/kernel/sound/` 路径。板上没有 `depmod`，需要在构建容器中将板端原模块目录与新增模块合并，使用 `depmod -b <合并目录> 6.1.99` 生成完整索引，再安装索引；须保留原蓝牙模块的依赖，不能只用音频模块生成索引覆盖系统索引。

原固件 `/etc/init.d/S03modules_init.sh` 会加载 `kernel/` 下的模块。完成索引更新后，使用 `modprobe snd-usb-audio` 检查依赖解析。本次已验证重启后声卡自动出现，约 20 秒恢复 SSH，屏幕仍为 480×640 RGB888，CST826 正常枚举，网络配置及原无线模块校验值不变。

该包仅针对本次核对的 SDK 内核和配置。更换内核版本或配置后应重新构建和验证；若后续重新打包 boot，继续包含 [wlk2802.dtsi](wlk2802.dtsi) 的屏幕与触摸配置。

## 录放音

实机声卡为 `Device`，同时提供播放与录音设备。硬件支持 S16_LE、44.1/48 kHz，播放为双声道，录音为单声道。使用 `plughw:CARD=Device,DEV=0` 可进行采样率和声道转换，已验证应用所需的 16 kHz 单声道录音与 24 kHz 单声道播放；当前唯一声卡下 `default` 也可用。扬声器到麦克风的声学回录测试通过，用户已确认三段左右声道测试音均可听到。

扬声器经 PAM8403 功放输出，`SPK_EN` 为 GPIO0_B4（GPIO12），播放时需要拉高。[speaker-amp.sh](speaker-amp.sh) 已安装为板端 `/usr/bin/hachimodock-speaker-amp`；功放默认关闭，应用尚未自动控制此 GPIO。

```sh
hachimodock-speaker-amp on
aplay -D plughw:CARD=Device,DEV=0 /path/to/audio.wav
hachimodock-speaker-amp off
```

测试使用 PCM 播放音量 `27,27`（−10 dB）、麦克风采集音量 `20`（+8 dB）、`Auto Gain Control=off`。混音器设置由 `amixer -c Device contents` 查看；这些是测试参数，不由功放脚本设置，也未新增混音器开机服务。重启后原固件的 ALSA 初始化使 PCM 音量为 `17,17`（−20 dB）、Mic 音量为 `20`、AGC 开启。

## 音量档位

2026-11-14 使用《相思河畔》逐档试听，用户确认以下设置。后续音量控制以 −20 dB 为日常档，−8 dB 为使用上限。

| 用途 | PCM 增益 | 左右声道原始值 | ALSA 显示 |
|---|---:|---|---:|
| 日常音量 | −20 dB | `17,17` | 46% |
| 稍响 | −14 dB | `23,23` | 62% |
| 最大使用音量 | −8 dB | `29,29` | 78% |

相邻档位相差 6 dB，输出信号幅度约翻倍。ALSA 百分比为控制刻度；使用上限按本次音乐试听确定，尚未进行仪器失真测量。当前工具只控制功放使能，音量上限尚未由应用强制限制。

例如，设置日常音量：

```sh
amixer -q -c Device cset 'name=PCM Playback Volume' 17,17
```

功能试验和测量脚本见 [sandbox/audio](../../sandbox/audio/README.md)。原模块目录及网络、DTB 校验记录保存在 `sdk/luckyfox-lyra/backups/pre-cm108b/`。需要回退时，卸载四个音频模块，删除新增的 `kernel/sound/` 目录并恢复备份中的模块索引；boot 分区无需恢复。
