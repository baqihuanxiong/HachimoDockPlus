## 项目简介

HachimoDock 的魔改版，将 ESP32P4 主控替换为幸狐 Lyra Zero W RK3506B，支持运行 Linux 系统。

## Luckfox Lyra Zero W 开发板网络配置

**开机选择板载天线**。保存以下内容到 `/etc/init.d/S35wifi-antenna`，并执行 `chmod +x /etc/init.d/S35wifi-antenna`，该脚本在无线驱动初始化前运行：

```sh
#!/bin/sh
[ "$1" = start ] || exit 0
[ -d /sys/class/gpio/gpio55 ] || echo 55 > /sys/class/gpio/export
echo out > /sys/class/gpio/gpio55/direction
echo 1 > /sys/class/gpio/gpio55/value
```

**保存网络配置**。编辑 `/etc/wpa_supplicant.conf`，替换名称和密码，并执行 `chmod 600 /etc/wpa_supplicant.conf`。

```conf
ctrl_interface=/var/run/wpa_supplicant
ap_scan=1
network={
    ssid="YOUR_SSID"
    psk="YOUR_PASSWORD"
    key_mgmt=WPA-PSK
}
```

**兼容性设置**。在 `/usr/bin/wifibt-init.sh` 的 `do_insmod()` 中，为 `aic8800_fdrv` 分支使用 `insmod "$1.ko" ps_on=0 he_on=0`。`he_on=0` 禁用 Wi-Fi 6 HE，使用 802.11n。

> 小米 AX3600 Mesh 下“有 IP 但无法通信”的问题，长期稳定性仍在观察。`ps_on=0` 是此前排查保留的省电关闭设置，其必要性尚未证实。

## WLK2802MIPI 屏幕适配

ST7701S 480×640 单 Lane DSI 屏幕与 CST826 触摸的设备树、构建和刷写记录见 [屏幕适配说明](board/lyra-zero-w-wlk2802/README.md)。第三版 RGB888 已完成 boot 刷写和读回校验，用户确认横屏应用的颜色、文字及方向正常；保留原内核、Wi-Fi 和 SSH 配置。CST826 曾出现 I²C 超时，在断电重新插牢排线后恢复；用户已确认横屏触控和输入发送正常，并收到模型回复。

## 参考资料

Go + LVGL 应用框架、PicoClaw 对话和构建部署方法见 [src/README.md](src/README.md)。

- [Luckfox Lyra Zero W 在线文档](https://wiki.luckfox.com/zh/Luckfox-Lyra/Luckfox-Lyra-Zero-W)
