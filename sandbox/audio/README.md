# CM108B USB 音频试验

开发板为 Lyra Zero W / RK3506B，Linux 6.1.99、Buildroot、SPI NAND。后续适配沿用 [板级说明](../../board/lyra-zero-w/README.md) 中已验证的 RGB888 屏幕固件基线。

2026-11-14 通过 `root@192.168.4.202` 初次检查：

- CM108B 枚举为 `0d8c:0012`、`C-Media Electronics Inc. USB Audio Device`，USB 路径为 `1-1.3`。
- 音频控制、播放和录音接口均已枚举，但没有绑定音频驱动；HID 接口已绑定 `usbhid`。
- `/proc/asound/cards` 为 `no soundcards`，`aplay -l` 与 `arecord -l` 无设备。
- `modprobe snd-usb-audio` 报告找不到模块；当前系统未提供可用的 USB Audio 驱动。

随后使用 SDK 202610 / GCC 10.3.1 构建并安装四个 USB Audio 模块，声卡注册为 `card 0: Device`。使用原内核，无需修改屏幕 DTB 或刷写 boot；构建与安装说明见 [板级音频说明](../../board/lyra-zero-w/usb-audio.md)。按 [底板原理图](../../hardware/SCH_底板_2026-11-07.pdf)，PAM8403 功放使能为 `SPK_EN`（GPIO0_B4 / GPIO12）。

复查设备识别：

```sh
lsusb
cat /proc/asound/cards
aplay -l
arecord -l
```

## 录放音试验

[pcm_probe.py](pcm_probe.py) 生成七秒测试 WAV：静音、左声道 997 Hz、静音、右声道 1499 Hz、静音、双声道 777 Hz、静音。每段一秒，测试音峰值 −18 dBFS，带 10 ms 淡入淡出。

主机生成 WAV 并传到板端临时目录：

```sh
python3 sandbox/audio/pcm_probe.py tone \
  sdk/luckyfox-lyra/artifacts/cm108b/speaker-probe.wav
```

板端以 48 kHz 单声道录制九秒，先开始录音，约半秒后播放测试音。播放时通过 `hachimodock-speaker-amp on` 开启功放，结束或出错时关闭。再将功放保持关闭、重复相同录放音作为对照。测试混音器参数为 PCM 音量 `27,27`、Mic 采集音量 `20`、AGC 关闭。

将 WAV 复制回主机后分析：

```sh
python3 sandbox/audio/pcm_probe.py analyze \
  sdk/luckyfox-lyra/artifacts/cm108b/loopback.wav
```

每 0.25 秒窗口计算去直流 RMS 和三个测试频率的幅度。以下为测试音稳定段的平均峰值幅度，单位 dBFS：

| 测试段 | 功放开启 | 功放关闭对照 |
|---|---:|---:|
| 左声道 997 Hz | −55.35 | −106.49 |
| 右声道 1499 Hz | −35.85 | −114.37 |
| 双声道 777 Hz | −49.47 | −100.16 |

录音无削顶，录放音日志无 overrun/underrun。功放关闭时测试频率信号显著下降，支持扬声器到麦克风的声学路径已打通；不同频率及摆放位置下的幅度不能用于判断左右声道音量平衡。这不是音质、最大声压或回声消除验收。

重启后再次播放相同测试 WAV，用户确认左声道、右声道和双声道三段测试音均能听到。

已验证原生 44.1/48 kHz 录音、48 kHz 双声道播放，以及 `plughw` 和 `default` 下的 16 kHz 单声道录音、24 kHz 单声道播放。测试日志、WAV、分析 JSON、模块包与配置保存在 `sdk/luckyfox-lyra/artifacts/cm108b/`，不纳入 Git。

## 音乐试听与音量

2026-11-14 将用户提供的《相思河畔》MP3 传到板端 `/tmp/`，使用板载 FFmpeg 解码为 48 kHz、S16_LE、双声道 WAV，通过 `hw:CARD=Device,DEV=0` 播放。

- −20 dB（PCM `17,17`，ALSA 46%）：完整播放约 3 分 11 秒，用户确认声音正常、适合作为日常音量。
- −14 dB（PCM `23,23`，ALSA 62%）：试听 30 秒，用户确认声音正常、稍响。
- −8 dB（PCM `29,29`，ALSA 78%）：再提升约一倍信号幅度、试听 30 秒，用户确认声音正常，可作为最大使用音量。

播放日志无 underrun，测试结束关闭功放，音量恢复到日常 −20 dB。日志位于产物目录中的 `music-playback.log`、`music-volume-minus14db.log` 和 `music-volume-minus8db.log`。这些结果为当前音乐和硬件条件下的主观试听记录，尚未测量失真上限。

## 重启验证

模块安装及依赖索引更新后，重启约 20 秒恢复 SSH，声卡自动注册。再次通过 16 kHz 录音和 24 kHz 播放测试，功放控制工具可正常切换 GPIO12，测试结束保持关闭。屏幕仍为 480×640p70、RGB888_1X24，CST826 仍为 `event0`。

原内核版本、Wi-Fi/SSH 配置及 AIC 模块校验值不变。启动前后 live DTB 的属性比较仅有两个以太网节点的 `local-mac-address` 变化，屏幕与触摸配置相同。详细记录见产物目录的 `post-reboot.txt`、`post-reboot.dtb` 和 `reboot-verification.json`。
