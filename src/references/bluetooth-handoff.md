# 蓝牙实机验证交接

更新日期：2026-11-15，Asia/Shanghai。用户已将开发板和串口移到另一台电脑，本会话停止板端操作。以下状态是断开前最后一次实测，下一会话须先确认设备是否重新上电。

## 给下一会话的任务

继续验证 Lyra Zero W 的经典蓝牙音箱发现、连接、实际发声，以及与 Wi-Fi 的关系。先读本文件，再确认串口；不要直接使用 Wi-Fi SSH 做模式切换。当前已找到默认模式与 BlueZ 接口兼容问题，不能再把问题仅归为未命名设备、手机占用或配对状态。

源码还有未提交、部分已暂存及未跟踪的修改。需要同步整个工作区的相关变更；仅在另一台电脑 `git clone` 不会得到这些修改。不要重置现有暂存区。账号密码不写入本文，串口用户为 `root`，密码由用户另行提供。

## 当前板端状态

| 项目 | 最后确认的状态 |
|---|---|
| 平台 | Luckfox Lyra Zero W，RK3506B，Buildroot，Linux 6.1.99，ARMv7 hardfloat |
| 无线模块 | USB AIC8800DC；USB 节点 `1-1.1`；蓝牙接口 0/1 使用 `btusb`，Wi-Fi 接口 2 使用 `aic8800_fdrv` |
| 蓝牙地址 | `54:01:4A:4B:EF:30`，BlueZ 5.77；BlueALSA 使用 `-p a2dp-source` |
| 启动模式 | 已注释 `/usr/bin/wifibt-util.sh` 第一条仅加载 `aic8800_fdrv.ko` 的 AIC8800DC 映射，并已重启测试 |
| Wi-Fi | 最后一次已执行 `rmmod aic8800_fdrv`；不可假定现在能 SSH。启动映射仍为经典蓝牙测试模式 |
| 网络地址 | 原 Wi-Fi `192.168.4.202`；USB 网口配置 `192.168.123.100/24`，此前未连接数据线、无 carrier |
| 天线 | 开机脚本 `/etc/init.d/S35wifi-antenna` 将 GPIO55 置高，使用板载天线 |
| 串口 | 1500000、8N1、无流控，3.3V TTL；板端 `/dev/ttyFIQ0`；此前 CH340 + WSL `/dev/ttyUSB0` 已可登录和执行命令 |
| 应用 | `/opt/hachimodock/current -> releases/91737e96fc1f`；服务 `/etc/init.d/S99hachimodock` |
| 配置 | 最后确认 `/opt/hachimodock/system/config-active -> config-versions/cfg-3533656131`；本轮未切换持久化音频输出到蓝牙 |
| 音频输出 | 此前用户设置为 S/PDIF、音量 51%；蓝牙测试直接使用 BlueALSA PCM，未改写用户 JSON |

板上已部署应用二进制 SHA256：

```text
91737e96fc1f04f3bfa961ad466d344e9ebb29646bc6287aa7fed5c95ecabeea
```

下面的 BlueZ 兼容修复尚未编译部署；不要把板上旧二进制当作包含了新修复。此前仅保留上述一个 release，根文件系统剩余约 37 MiB。部署新 release 前核对空间，并保留配置和用户 assets。

## 已证实的结果

1. 默认启动方式下，BLE 设备可发现，但 S300 和 S1000MKII 均无法被经典 Inquiry 找到；手机可见，用户已确认音箱配对状态。名称丢失不是这个现象的完整解释。重置 HCI、不同 Inquiry 模式、临时使用 SDK `aic_btusb` 均未解决，临时驱动试验已恢复为 `btusb`。
2. 按 [Luckfox 官方说明](https://wiki.luckfox.com/Luckfox-Lyra/Luckfox-Lyra-Zero-W/WIFI-BT/#2-bluetooth-test) 注释第一条 AIC8800DC 映射并重启后，Wi-Fi 不再加载，但经典发现恢复。直接 `hcitool inq` 可见 `FC:E8:06:30:C7:F0`，BlueZ 显示 **EDIFIER S1000MKⅡ**。S300 尚未在此模式复测。
3. S1000MKⅡ 已成功配对、信任。板上 `Connect` / `Disconnect` D-Bus 方法额外要求字符串 `ADDR_TYPE`，随附 `bluetoothctl connect MAC` 却按上游无参数签名调用，因而出现 `org.freedesktop.DBus.Error.UnknownMethod`。本地 SDK 的 Rockchip BlueZ 补丁证实了这点。
4. 直接调用 `Device1.ConnectProfile`，选择 Audio Sink UUID `0000110b-0000-1000-8000-00805f9b34fb` 后连接成功。`bluealsa-aplay -L` 列出 SBC、S16_LE、双声道、48000 Hz 的 playback PCM。
5. 在未加载 Wi-Fi 驱动时，`aplay` 曾成功播放 2 秒低幅度测试音，退出码为 0。用户当时未留意，**尚未确认音箱实际发声**。
6. 随后手动 `insmod /lib/modules/aic8800_fdrv.ko ps_on=0 he_on=0`，`wlan0` 出现；最初蓝牙仍连接，稍后断开。重连报 `br-connection-page-timeout`，重放时报 BlueALSA `PCM not found`。未进一步连接 Wi-Fi AP，不能声称已经验证联网或共存。
7. 卸载 Wi-Fi 驱动后，本轮 Inquiry 未再发现音箱，重连仍超时。尚未在卸载后重启复测，不能将热卸载等同于恢复了干净的经典蓝牙模式。下一步先重启到既有经典测试模式。

**目前只证明指定启动方式能恢复经典发现、配对和软件侧 A2DP 播放；没有证明实际发声、Wi-Fi / 经典蓝牙共存、应用页面完整闭环或重启自动重连。**

## 配对记录持久化问题

系统 `/data -> userdata` 是相对符号链接，但镜像此前没有 `/userdata`。BlueZ 编译时将状态保存在 `/data/lib/bluetooth`，日志出现写缓存失败。

本轮已在板上创建 `/userdata/lib/bluetooth` 并将蓝牙状态目录设为 0700；通过 untrust / trust 触发写入后，音箱 `info` 文件已存在。但检查未发现 `[LinkKey]` 节，首次配对时的密钥可能没有持久化。不要输出整个 `info` 文件或密钥。恢复模式后，可能需要对这台测试音箱重新配对；无需清空其他设备记录。

源码 `tools/S45hachimodock-audio` 已补上此镜像 `/data -> userdata` 布局的目录准备，但新版脚本尚未安装到板上。目录本身已经创建并位于持久化根文件系统。

## 本轮新增、尚未部署的修复

- `internal/platform/bluez/device.go`、`device_test.go`（未跟踪）：共享连接封装；音频使用 `ConnectProfile(Audio Sink)`；普通连接和断开先调用标准接口，仅在 `UnknownMethod` 时使用 Rockchip 的 `string:auto` 参数。
- `internal/platform/audio/output.go`：蓝牙输出改用上述音频连接接口。
- `internal/platform/connectivity/bluetooth.go`：页面连接音频设备时选择 Audio Sink；其他设备连接和断开复用兼容接口。
- `tools/S45hachimodock-audio`：创建缺失的持久化蓝牙状态目录。

已通过 `gofmt`、以下相关 Go 测试及音频启动脚本 `sh -n`：

```sh
cd src
go test ./internal/platform/bluez ./internal/platform/audio ./internal/platform/connectivity
sh -n tools/S45hachimodock-audio
```

**尚未执行新修复的全量检查、ARM 构建和部署。** 原主机 Go 在 `/home/lwshen/.local/go/bin`，默认 shell PATH 不包含它；另一台主机按自己的环境处理。已有扫描缓存、交互式扫描进程生命周期、名称解析和 UI 修复已在前一版部署；工作区还有 SMB/assets/全局音频配置、文档合并等较大修改，勿覆盖。

SDK 证据：`sdk/luckyfox-lyra/source/.repo/projects/buildroot.git`，提交 `48b41188a3a7ef5d6d49d57e1d5a6f4dc59fbf4a` 的 `package/bluez5_utils/0001-bluez-modified-only-for-rockchip.patch`。该补丁支持 `auto`、`bredr`、`public`、`random`。`Connect string:auto` 可能尝试其他不支持的 profile 并报错，因此音频路由明确调用 Audio Sink。

## 下一会话的验证顺序

1. 在新电脑确认串口实际设备名和权限，1500000 登录，执行 `hostname`、`uname -r`、`cat /proc/uptime`。检查是否已经断电重启、当前模块和映射；不要假定旧 COM3 / BUSID / `/dev/ttyUSB0` 仍适用。
2. 若仍是刚热卸载 Wi-Fi 的状态，先 `sync; reboot`，串口记录启动过程。现有经典模式会加载 `aic_load_fw` 而不自动加载 Wi-Fi。不要立刻手动加载 Wi-Fi。
3. 确认音箱处于蓝牙输入和配对模式，手机暂时关闭蓝牙。在交互式 `bluetoothctl` 中 `power on`、`scan on`，验证 S1000MKⅡ 或 S300 的实际名字。
4. 检查测试音箱的配对记录。若缺密钥而无法认证，仅删除这台测试音箱的旧记录，再配对、信任；配对前确认持久化目录已就绪。随后用下面的 D-Bus 命令连接音频。
5. 低音量测试并由用户明确确认听到声音，再验证断开、重连和重启后配对记录保留。
6. 构建、部署源码兼容修复，在系统配置页面检查已配对音箱名称、连接 / 断开、蓝牙音频输出选择及 JSON 持久化。不要因 Wi-Fi 不通就重刷镜像或替换内核。
7. Wi-Fi / 经典蓝牙共存仍需单独调查，先读官方限制及本 SDK USB 驱动 / 固件初始化流程。现有试验不足以认定硬件绝不支持共存，也不足以声称只改一个参数就能共存。不要套用 SDIO 或其他 AIC 型号的固件。

板端可直接执行的音频连接与低幅度测试：

```sh
/etc/init.d/S45hachimodock-audio start
dbus-send --system --dest=org.bluez --type=method_call --print-reply \
  --reply-timeout=15000 \
  /org/bluez/hci0/dev_FC_E8_06_30_C7_F0 \
  org.bluez.Device1.ConnectProfile \
  string:0000110b-0000-1000-8000-00805f9b34fb
bluealsa-aplay -L
sox -n -r 48000 -c 2 -b 16 /tmp/hachimodock-bt-test.wav \
  synth 2 sine 660 vol 0.03
aplay -D 'plug:SLAVE="bluealsa:DEV=FC:E8:06:30:C7:F0,PROFILE=a2dp"' \
  /tmp/hachimodock-bt-test.wav
```

系统附带的 `bluetoothctl connect MAC a2dp-sink` 也不支持，实测报参数过多。非交互 `bluetoothctl` 在部分错误后不会及时退出，脚本应外加 `timeout`。串口传长命令时最好分段发送，先 `stty -echo` 可减少回显干扰；此前 `stty -F /dev/ttyFIQ0 -a` 的 115200 显示伴有 ioctl 错误，不能据此覆盖官方串口 1500000 设置。

## 恢复原 Wi-Fi 启动模式

仅在决定恢复默认模式时，通过串口执行以下步骤；会重新回到此前 Wi-Fi / BLE 模式，不代表经典蓝牙问题已修复。

```sh
sha256sum /root/hachimodock-recovery/wifibt-util.before-classic.sh
# 必须与下方原始 SHA256 一致后再复制。
cp -p /root/hachimodock-recovery/wifibt-util.before-classic.sh /usr/bin/wifibt-util.sh
sync
reboot
```

原始文件 SHA256：

```text
658c067145553d5c939070690dbd5fc5711964c13497a9e51c23fe0bc4961747
```

当前经典模式脚本 SHA256：

```text
bbb871bded6894e5da438c1a8900ecf2f7b09a306f4013c25b10e9b0a538eab9
```

唯一启动脚本改动是注释原第 121 行：

```text
# Aicsemi AIC8800DC a69c:88dc aic8800_fdrv.ko
```

后面同 ID 的 `aic_load_fw.ko  aic8800_fdrv.ko` 映射仍在，工具实际只返回首个模块。重启会清空 `/var/run` 的芯片映射缓存；仅热改文件不能保证缓存同步。

## 本地主机证据和其他未完成事项

原工作区 `src/build/diagnostics/`（被 Git 忽略，不会随普通 Git 同步迁移）包含：

- `classic-mode-boot.log`、`classic-mode-discovery.log`、`classic-mode-pairing.log`：启动、发现与配对。
- `classic-mode-dbus.log`、`classic-mode-a2dp.log`：接口签名、A2DP PCM。
- `classic-mode-wifi-load.log`、`classic-mode-wifi-unload.log`：Wi-Fi 模块试验。
- `bluez-rockchip.patch`、`wifibt-util.original.sh`、`wifibt-util.classic.sh`：SDK 接口证据及启动脚本原件 / 测试版。
- `serial-console.py`：原主机临时串口命令工具，依赖 pyserial；不是产品依赖。

S/PDIF 排查由用户暂停：选择 S/PDIF 后 ALSA 曾处于 RUNNING、48k 双声道，GPIO13 高；用户测得 U8 供电和使能均为 3.3V，同轴端约 200mV 直线无波形。尚未定位故障。底板原理图第 2 页信号链为 CM108B pin1 SPDIFO → U8 SN74LVC1G126 → 电阻 / 电容网络 → CN4 pin4，同轴地 CN4 pin1。不要把蓝牙进展当作 S/PDIF 已修好。
