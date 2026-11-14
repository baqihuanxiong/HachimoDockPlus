## 项目简介

HachimoDock 的魔改版，将 ESP32P4 主控替换为幸狐 Lyra Zero W RK3506B，支持运行 Linux 系统。

## 项目结构

| 目录 | 内容 |
|---|---|
| [board/lyra-zero-w](board/lyra-zero-w/README.md) | Lyra Zero W 板级适配：屏幕、触摸、网络及固件工具 |
| [src](src/README.md) | Go + LVGL 应用、PicoClaw 对话、构建与部署 |
| [hardware](hardware/) | 底板原理图、引脚定义及器件资料 |
| [sandbox](sandbox/README.md) | 按功能组织的开发试验、验证与排查记录 |
| `sdk/` | 本机 SDK、固件备份和构建产物，不纳入 Git |

后续支持其他开发板时，在 `board/<开发板名>/` 下添加对应的板级适配文件；功能试验放在 `sandbox/<功能名>/` 下，并注明开发板与固件基线。

## 当前开发板

Lyra Zero W 的 WLK2802MIPI 屏幕与 CST826 触摸已完成实机验证，Go + LVGL 主程序提供 PicoClaw 和 Wi-Fi / 蓝牙设置，支持开机自启。CM108B 已支持 USB Audio 录放音，通过模块扩展现有内核，保留已验证的 RGB888 屏幕配置；详见 [USB 音频说明](board/lyra-zero-w/usb-audio.md)。

配置与构建方法见 [板级说明](board/lyra-zero-w/README.md)，无线网络设置见 [网络配置](board/lyra-zero-w/wifi.md)。

## 参考资料

- [Luckfox Lyra Zero W 在线文档](https://wiki.luckfox.com/zh/Luckfox-Lyra/Luckfox-Lyra-Zero-W)
