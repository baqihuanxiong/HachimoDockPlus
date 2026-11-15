# Lyra Zero W 网络配置

开机选择板载天线：保存以下内容到 `/etc/init.d/S35wifi-antenna`，并执行 `chmod +x /etc/init.d/S35wifi-antenna`，该脚本在无线驱动初始化前运行。

```sh
#!/bin/sh
[ "$1" = start ] || exit 0
[ -d /sys/class/gpio/gpio55 ] || echo 55 > /sys/class/gpio/export
echo out > /sys/class/gpio/gpio55/direction
echo 1 > /sys/class/gpio/gpio55/value
```

编辑 `/etc/wpa_supplicant.conf`，替换名称和密码，并执行 `chmod 600 /etc/wpa_supplicant.conf`。

```conf
ctrl_interface=/var/run/wpa_supplicant
ap_scan=1
network={
    ssid="YOUR_SSID"
    psk="YOUR_PASSWORD"
    key_mgmt=WPA-PSK
}
```

在 `/usr/bin/wifibt-init.sh` 的 `do_insmod()` 中，为 `aic8800_fdrv` 分支使用 `insmod "$1.ko" ps_on=0 he_on=0`。`he_on=0` 禁用 Wi-Fi 6 HE，使用 802.11n。

小米 AX3600 Mesh 下“有 IP 但无法通信”的问题，长期稳定性仍在观察。`ps_on=0` 是此前排查保留的省电关闭设置，其必要性尚未证实。

## 经典蓝牙音频与默认 BLE 模式

Luckfox 的 [Zero W 无线说明](https://wiki.luckfox.com/Luckfox-Lyra/Luckfox-Lyra-Zero-W/WIFI-BT/#2-bluetooth-test) 指出，默认 Wi-Fi / 蓝牙协作使用 BLE 模式，连接传统蓝牙音箱需切换模式。`bluetoothctl show`、HCI supported features 或 BlueALSA 已注册 Audio Source，都不能单独证明当前无线初始化路径支持经典蓝牙发现与 A2DP。

2026-11-15 在当前 SDK 202610 / Linux 6.1.99 板端检查：BLE 广播可发现；EDIFIER S300 与 S1000MKII 在用户确认的配对状态下，自动扫描、经典专项扫描及直接 HCI Inquiry 均未发现。标准、RSSI、扩展 Inquiry 模式结果一致；仅重新初始化蓝牙和临时对比 SDK 自带 `aic_btusb.ko` 未恢复发现。驱动对比已回滚，Wi-Fi 配置保持原样。现阶段不能将问题归为页面名称解析，也不能声称蓝牙音频已通过实机验收。

官方步骤是注释 `/usr/bin/wifibt-util.sh` 中仅加载 `aic8800_fdrv.ko` 的 AIC8800DC 映射行，再重启。当前板端还有一条同 ID 的 `aic_load_fw.ko  aic8800_fdrv.ko` 后备映射；查询函数取首个匹配，`wifibt_module` 实际只返回第一模块，修改会改变 Wi-Fi 的初始化路径。不能在唯一连接为 Wi-Fi 时直接应用该改动并重启。先接通 USB 有线网络或串口恢复通路，再核对模式切换后的经典发现、配对、A2DP，以及 Wi-Fi 能否同时工作。当前 `usb0` 配有 `192.168.123.100/24`，需要主机侧 USB 数据连接和同网段地址；链路未接通时该地址不可用。

建立串口恢复通路后，已按官方步骤注释映射并重启：可发现 EDIFIER S1000MKⅡ、配对并通过 BlueALSA 播放测试音，实际发声尚待用户确认。板端 Rockchip BlueZ 的 `Connect` / `Disconnect` 比附带客户端额外要求 `ADDR_TYPE`，音频使用 `ConnectProfile(Audio Sink)` 已连接成功。随后热加载 Wi-Fi 驱动虽出现 `wlan0`，蓝牙连接稍后断开且重连超时；热卸载后尚未重启复测。完整交接、当前模式与恢复命令见 [蓝牙实机验证交接](../../src/references/bluetooth-handoff.md)。

恢复原模式时还原该映射行并重启；不要删除用户的 Wi-Fi 凭据、蓝牙配对记录或替换整个系统镜像。尚未验证本 SDK 的 USB 无线方案能在当前网络配置下同时保持 Wi-Fi 和经典蓝牙音频；SDIO 的固件及其他 AIC 型号的补丁不能直接替换到本板 USB 模块。
