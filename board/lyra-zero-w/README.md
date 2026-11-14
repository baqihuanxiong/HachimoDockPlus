# Luckfox Lyra Zero W 板级适配

本目录统一维护 Lyra Zero W / RK3506B 的设备树、固件工具和配置说明。目标为 `Luckfox_Lyra_SDK_202610`、SPI NAND、Buildroot，当前内核为 Linux 6.1.99。

## 文件与当前状态

| 文件 | 用途 |
|---|---|
| [wlk2802.dtsi](wlk2802.dtsi) | WLK2802MIPI / ST7701S 屏幕及 CST826 触摸配置 |
| [repack_fit.py](repack_fit.py) | 保留内核和启动图片，替换 FIT 与 resource 中的两份 DTB |
| [wifi.md](wifi.md) | 板载天线、无线网络及驱动参数配置 |
| [usb-audio.config](usb-audio.config)、[build-usb-audio.sh](build-usb-audio.sh) | CM108B USB Audio 模块配置与构建 |
| [Dockerfile](Dockerfile) | 使用 SDK 工具链的内核模块构建环境 |
| [speaker-amp.sh](speaker-amp.sh) | PAM8403 扬声器功放使能控制 |

屏幕与触摸已完成实机验证；当前固件屏幕显示 Linux 控制台。Go + LVGL 横屏应用曾验证颜色、文字、触控和输入发送正常，启动方式见 [应用说明](../../src/README.md)。CM108B 已通过标准 USB Audio 模块注册为 ALSA 声卡，录放音及扬声器到麦克风的声学测试通过；构建、部署及功放控制见 [USB 音频说明](usb-audio.md)。

功能试验和排查过程放在 [sandbox](../../sandbox/README.md) 对应目录；本目录保留可复用的板级适配。屏幕历史记录见 [sandbox/display](../../sandbox/display/README.md)。

## 屏幕与触摸配置

- ST7701S：480×640、单 Lane DSI、RGB888、8 bpc，`bus-format = RGB888_1X24`、`0x3A = 0x77`。
- 像素时钟 24 MHz，HFP/HSYNC/HBP 为 10/4/20，VFP/VSYNC/VBP 为 8/4/14，刷新率约 70 Hz；Lane rate 配置为 800 Mbps。
- 保留官方 `simple-panel-dsi` 驱动及完整初始化命令；使用软件复位，禁用多余的 Goodix 触摸和 Waveshare 背光配置。
- CST826 位于 I²C2 `0x38`，复用 SDK `edt_ft5x06` 的 `focaltech,ft6236` 协议兼容配置及无 IRQ 轮询；此配置不表示实际芯片为 FocalTech。原生触摸范围为 480×640，X/Y 反向、不交换坐标。
- 屏幕通过 22Pin 转接排线连接，未确认外部复位、触摸中断及背光 PWM 接线，不额外配置这些 GPIO。接线依据见 [裸屏定义](../../hardware/references/schematics/WLK2802_Pinout.jpg) 和 [转接图](../../hardware/references/schematics/WLK2802_FPC.jpg)；[底板引脚表](../../hardware/Pinout.md) 的 SPI 屏分配不适用于此 DSI 连接。
- LVGL 在应用内旋转为 640×480 横屏，设备树保持原生 480×640。触摸出现 I²C 超时时，曾通过断电重新插牢排线恢复。

## 应用到 SDK

准备 SDK 源码后，在 SDK 根目录执行 `./build.sh lunch`，选择 **Lyra Zero W → SPI_NAND → Buildroot**，对应 `luckfox_lyra_zero-w_buildroot_spinand_defconfig`。内核配置为 `rk3506_luckfox_defconfig` 加 `rk3506-display.config`。

从项目根目录复制配置：

```sh
cp board/lyra-zero-w/wlk2802.dtsi \
  sdk/luckyfox-lyra/source/kernel-6.1/arch/arm/boot/dts/
```

在 `kernel-6.1/arch/arm/boot/dts/rk3506b-luckfox-lyra-zero-w.dts` 末尾包含一次：

```dts
#include "wlk2802.dtsi"
```

再在 SDK 根目录执行 `./build.sh kernel`，输出为 `rockdev/boot.img`。SDK 默认已有 `CONFIG_DRM_PANEL_SIMPLE=y`、`CONFIG_DRM_PANEL_SIMPLE_OF_ONLY=y` 和 `CONFIG_TOUCHSCREEN_EDT_FT5X06=y`。屏幕公共配置位于 `rk3506-luckfox-lyra-dsi.dtsi`。

**后续声卡等适配须保留上述屏幕和触摸配置**，并保留现有 Wi-Fi、SSH 及无线模块。固件基线是已刷入并读回校验的 RGB888 第三版，不能回退到官方 800×1280 双 Lane 屏幕或早期 RGB565 配置。

## 固件基线与重打包

以下备份与产物保存在被 Git 忽略的 `sdk/luckyfox-lyra/` 下：

| 路径（相对此 SDK 目录） | 用途 |
|---|---|
| `backups/pre-wlk2802/boot.img`、`uboot.img` | 屏幕适配前的原始分区备份 |
| `artifacts/wlk2802/boot-wlk2802-v3.img` | 已验证的 RGB888 屏幕固件，8,110,080 字节 |
| `artifacts/wlk2802/wlk2802-rgb888.dtb` | 第三版 DTB |
| `artifacts/wlk2802/boot-wlk2802-v3.manifest.json` | 内核、DTB、启动图片和固件校验值 |

第三版固件 SHA-256：`42df53ba56b0f82c44bbe0e6560f9ad8843b14d264a5266dbbc9b646fb383adb`。

仅重打包屏幕 DTB 时，从项目根目录执行：

```sh
python3 board/lyra-zero-w/repack_fit.py \
  sdk/luckyfox-lyra/backups/pre-wlk2802/boot.img \
  sdk/luckyfox-lyra/artifacts/wlk2802/wlk2802-rgb888.dtb \
  /tmp/boot-wlk2802-rgb888.img
```

脚本只写本地镜像和校验清单，限定只允许屏幕/触摸属性变化，保留内核及启动图片，拒绝签名 FIT、校验错误和超出 DTB 空间的替换。它同时更新 FIT 独立 DTB 和 resource 中的 `rk-kernel.dtb`，因为 U-Boot 可能以后者覆盖前者。**此脚本不能用于替换内核或添加声卡驱动**；后续重新构建内核时也必须保留两处 DTB 的屏幕配置。

板上没有 `/boot` 目录，更新固件需要使用与 SDK 匹配的 NAND 刷写工具更新 boot 分区，并保留可恢复备份。

## 板端检查

```sh
cat /sys/kernel/debug/dri/0/summary
cat /proc/bus/input/devices
dmesg | grep -Ei 'dsi|panel|touch|ft5'
```

显示应为 480×640、RGB888_1X24；触摸设备号以实际枚举为准。新增功能后复查冷启动、颜色与文字、横屏应用触控，以及 Wi-Fi 和 SSH。
