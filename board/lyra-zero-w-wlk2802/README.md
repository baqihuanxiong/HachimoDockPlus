# Lyra Zero W：WLK2802MIPI 屏幕适配

适用于本项目已经在 ESP32-P4 上使用的 **WLK2802MIPI / ST7701S、480×640、单 Lane DSI** 屏幕，以及用户提供的 22Pin 转接排线。适配文件为 [wlk2802.dtsi](wlk2802.dtsi)，目标 SDK 为 `Luckfox_Lyra_SDK_202610`。

当前状态（2026-11-12）：**第三版 RGB888 显示已通过实机确认，重新插牢排线后触摸已恢复枚举**。第二版 RGB565 虽能出图，但运行 LVGL 后用户反馈绿色偏色、文字失真；第三版同时改为 DSI RGB888、RGB888_1X24、8 bpc、`0x3A=0x77` 后，用户确认颜色、文字和横屏方向正常。原生时序仍为 480×640、约 70 Hz，Go + LVGL 应用软件旋转为 640×480。

触摸曾出现冷启动、完整断电和重新绑定驱动后仍 I²C 超时，系统没有 CST826 输入设备。解绑 I²C 控制器并将 GPIO0_A0/A1 设为输入，读到 SDA=1、SCL=0；开漏时钟恢复后不变，随后已恢复 I²C 引脚及驱动。用户在断电状态重新插牢 22Pin 排线后，驱动成功绑定 `2-0038`，触摸恢复为 `/dev/input/event0`，电源键变为 event1。最新横屏应用中用户已确认坐标、页面切换、屏幕键盘输入和发送正常，并收到 PicoClaw 真实模型回复；采集到 13 次按下及 13 次抬起。当前不配置未经接线确认的复位 GPIO。

## 本次构建与刷写记录

采用现有 boot.img 的原始 DTB 作为基底，仅覆盖本文件所述显示和触摸属性，保留原 Linux 6.1.99 内核及 rootfs。所有非显示/触摸的 DTB 属性均经脚本比较限制；新镜像没有更新 U-Boot 分区。

本机产物（位于已忽略的 `sdk/` 目录）：

- 原始 12 MiB boot 备份：`sdk/luckyfox-lyra/backups/pre-wlk2802/boot.img`，SHA-256 `525880aa0df50286cb8a9d2798dca9af464375c7bee9c6246ada475e5d1970e4`。
- 原始 4 MiB U-Boot 备份：`sdk/luckyfox-lyra/backups/pre-wlk2802/uboot.img`。
- 第二版镜像：`sdk/luckyfox-lyra/artifacts/wlk2802/boot-wlk2802-v2.img`，8,110,080 字节，SHA-256 `9921620d6d18d05911d2b7bc77022595c120badcc45f5db7b9874a9a1e9712e3`。
- 当前第三版：`sdk/luckyfox-lyra/artifacts/wlk2802/boot-wlk2802-v3.img`，8,110,080 字节，SHA-256 `42df53ba56b0f82c44bbe0e6560f9ad8843b14d264a5266dbbc9b646fb383adb`。已完成 NAND 写入、同长度读回并核对上述哈希，重启后 DRM 确认为 RGB888_1X24；对应 DTB 为 `wlk2802-rgb888.dtb`。
- 对应 `.manifest.json` 包含内核、启动图片及 DTB 校验值，`flash-v2.log` 记录写入和读回结果。
- `original-patched.dts` 是以原固件反编译设备树为基础的本次构建输入，`wlk2802.dtb` 是编译结果。

[repack_fit.py](repack_fit.py) 用于本布局的本地重打包。它会拒绝签名 FIT、原镜像哈希错误、非显示/触摸属性变化，以及超出原 DTB 空间的替换。脚本同时替换 FIT 独立 DTB 和 resource.img 的 `rk-kernel.dtb`，更新资源 SHA-1 与 FIT SHA-256；原内核和两张 BMP 图片保持逐字节一致。它不执行刷写。

```bash
python3 board/lyra-zero-w-wlk2802/repack_fit.py \
  sdk/luckyfox-lyra/backups/pre-wlk2802/boot.img \
  sdk/luckyfox-lyra/artifacts/wlk2802/wlk2802-rgb888.dtb \
  sdk/luckyfox-lyra/artifacts/wlk2802/boot-wlk2802-v3.img
```

U-Boot 源码中的 `arch/arm/mach-rockchip/fit_misc.c` 在 `USING_KERNEL_DTB` 下会用预先处理的内核 DTB 覆盖 FIT 的独立 DTB，因此不能仅替换独立 DTB。第一版据此被第二版替代。

最终实机再次确认 `/etc/wpa_supplicant.conf`、`S35wifi-antenna`、`wifibt-init.sh`、`sshd_config` 及三个 AIC 无线模块 SHA-256 均与刷写前一致。

第二版第一次重启时曾出现从本机 SSH 无法握手的情况；用户串口登录后连接恢复。板端日志显示 sshd 在开机约 4 秒监听、Wi-Fi 约 12 秒获得原 IP，随机数发生器约 3.6 秒完成初始化，用户没有在串口执行联网命令。随后再次自动重启，本机 SSH 约 **19.9 秒自行恢复**，没有串口登录依赖的证据。第一次连接延迟的确切原因尚未定位，不能据此认定显示设备树或串口登录导致网络恢复。

复核资料位于产物目录中的 `post-boot-v2.txt`、`running-verified.dtb`、`display-touch-verification.json`、`network-and-modules-after.txt` 与 `reboot-verification.json`。保留原始备份，供后续恢复使用。

## 实机核对结果

- 开发板：`root@192.168.5.156`，Linux 6.1.99，SPI NAND 启动。
- 原显示模式：800×1280，2 Lane，RGB888，像素时钟约 70 MHz。
- 当前模式：480×640p70，RGB888_1X24，实际像素时钟 24 MHz，DSI 链路约 798 Mbps × 1。
- 早期触摸验证：用户点按后 `/dev/input/event0` 在 50 秒内产生了 1107 条坐标或跟踪 ID 事件，包括抬起事件；后续排线复测过程见开头说明。
- 触摸在 I²C2 的 **0x38**，实读 `0xAA = 0x11`，符合项目原固件的 CST826 身份判断；`0xA3 = 0x00`，不是原固件识别的 FT6336U ID `0x64`。0x15 没有应答，不用于当前配置。
- 上述结果证明当前驱动能读取报点，不代表已完成四角定位、多指和休眠恢复验收。
- 刷写后 40 秒测试收到 358 条坐标/跟踪事件，43 次按下和 43 次抬起；实际输入范围 X=0～479、Y=0～639，采样覆盖 X=21～462、Y=26～639。

## 接线依据

参考 [裸屏定义](../../hardware/references/schematics/WLK2802_Pinout.jpg)、[15Pin/22Pin 转接图](../../hardware/references/schematics/WLK2802_FPC.jpg) 和 [开发板原理图](../../hardware/references/schematics/Luckfox-Lyra-Zero-W.pdf)。

第一张图是 **24Pin 裸屏 FPC**，不是开发板的 22Pin 插座。它仅引出 D0+/D0− 与 CLK+/CLK−，确认只使用一条数据 Lane。

第二张图中 22Pin 侧的重要信号如下：

| 22Pin | 信号 | 本板用途 |
|---|---|---|
| 1 | VCC | 由开发板屏幕接口供电 |
| 2 | SDA0 | I²C2 SDA，GPIO0_A0 |
| 3 | SCL0 | I²C2 SCL，GPIO0_A1 |
| 5、6 | 未连接 | 不据此配置 RESET、触摸 INT 或背光 PWM |
| 14、15 | DSI CLK+/− | DSI 时钟 |
| 20、21 | DSI D0+/− | 屏幕实际使用的数据 Lane |

没有屏幕背板完整原理图，不能据这些图确定背板内部的复位和背光控制电路。候选配置采用 DSI 软件复位，保留已亮的屏幕侧背光。它不绑定额外 GPIO，也不将开发板 LCD_IO0/1 假定为屏幕复位或 PWM。

`hardware/Pinout.md` 中 SPI LCD、I²C0 和 FT6336U 的分配属于另一套底板方案，不适用于这里的 DSI 直连接法。

## 配置内容

- 保留官方 `simple-panel-dsi`，不新增显示驱动。
- DSI 改成 1 Lane、RGB888，`0x3A = 0x77`，Lane rate 采用原 ESP32 BSP 的 800 Mbps。ESP32 的 RGB565 配置在本板出现偏色与文字失真，不能直接沿用。
- 从项目原 BSP 移植完整厂商命令表，以及原 ST7701 驱动在表前发送的命令页选择、`0x36` 和 `0x3A`。无已知硬件复位连接，额外增加软件复位和 120 ms 等待；Display On 后增加 20 ms 等待。
- 时序：480×640，HFP/HSA/HBP=10/4/20，VFP/VSA/VBP=8/4/14，像素时钟 24 MHz。
- 上述总时序为 514×666，理论刷新率约 **70.1 Hz**，与原 ESP32 v3 配置保持一致。不要将其标注为 60 Hz；实际像素时钟以 DRM summary 为准。
- 移除继承的官方 10.1 英寸物理尺寸，不猜测本屏有效显示区域尺寸。
- 禁用多余的 Goodix 触摸、Waveshare I²C 背光及未连接的外部 PWM 背光配置。
- 触摸保留 0x38，尺寸改为 480×640，X/Y 均反向，不交换坐标；这是根据原 ESP32 固件的 CST826 坐标映射得出的竖屏起点，需上板检查四角。

触摸的 `compatible = "focaltech,ft6236"` 是**当前板子的协议兼容配置**，用来复用 SDK `edt_ft5x06` 中两触点、六字节记录的解析器，不表示芯片实际是 FocalTech。该 SDK 驱动已有无 IRQ 轮询，因此本阶段不需要另写 CST826 驱动，也不需要猜测 INT 接线。正式提交上游设备树时应增加真实的 CST826 绑定与驱动匹配支持，不能把此兼容配置视作上游硬件描述。

源代码依据：

- [原 BSP 初始化命令及时序](../../HachimoDock/firmware/components/esp32_p4_wifi6_touch_lcd_4_3/esp32_p4_wifi6_touch_lcd_4_3.c)
- [原 ST7701 驱动](../../HachimoDock/firmware/components/esp_lcd_st7701/esp_lcd_st7701_mipi.c)
- [CST826 身份判断与轮询](../../HachimoDock/firmware/main/pet_p4_touch.c)
- [原固件坐标映射](../../HachimoDock/firmware/main/pet_p4_touch_core.c)
- SDK `kernel-6.1/drivers/input/touchscreen/edt-ft5x06.c`，含无 IRQ 轮询及 `focaltech,ft6236` 两触点配置。

SDK 的 `panel-simple` 在没有外部 backlight 属性时会自动创建 DCS 背光设备。它的存在不能证明屏幕支持 DCS 调光；本轮没有验证调光，也不依赖读取该设备的 brightness 属性判断背光状态。

## 应用到 SDK

以下命令在项目根目录执行，只准备源码，不刷写设备：

```bash
mkdir -p sdk/luckyfox-lyra/source
tar -xzf sdk/luckyfox-lyra/Luckfox_Lyra_SDK_202610.tar.gz \
  -C sdk/luckyfox-lyra/source
cd sdk/luckyfox-lyra/source
.repo/repo/repo sync -l
./build.sh lunch
```

在菜单中选择 **Lyra Zero W → SPI_NAND → Buildroot**，对应配置为 `luckfox_lyra_zero-w_buildroot_spinand_defconfig`。内核配置是 `rk3506_luckfox_defconfig` 加 `rk3506-display.config`，设备树是 `rk3506b-luckfox-lyra-zero-w.dts`。

回到项目根目录，复制适配文件：

```bash
cp board/lyra-zero-w-wlk2802/wlk2802.dtsi \
  sdk/luckyfox-lyra/source/kernel-6.1/arch/arm/boot/dts/
```

在 SDK 的 `kernel-6.1/arch/arm/boot/dts/rk3506b-luckfox-lyra-zero-w.dts` **文件末尾**增加一次：

```dts
#include "wlk2802.dtsi"
```

202610 SDK 的屏幕公共配置已拆到 `rk3506-luckfox-lyra-dsi.dtsi`。不要根据旧文档修改不存在的 `rk3506-luckfox-lyra-ultra.dtsi`，也不要同时用 `luckfox-config` 切换回官方屏幕配置。

在 SDK 根目录构建：

```bash
./build.sh kernel
```

默认已有 `CONFIG_DRM_PANEL_SIMPLE=y`、`CONFIG_DRM_PANEL_SIMPLE_OF_ONLY=y`、`CONFIG_TOUCHSCREEN_EDT_FT5X06=y`。候选方案只改设备树，不增加模块；本次不需要构建整个 Buildroot rootfs。首次构建仍需按官方文档准备主机依赖与 SDK 交叉工具链。

输出 `rockdev/boot.img` 是打包的内核和设备树。板上没有 `/boot` 目录，不能靠往根文件系统复制一个 `.dtb` 生效。部署时先保存可恢复的原固件和现有 Wi-Fi 配置，再使用该 SDK 对应的官方工具更新 **boot 分区**；这里不提供对 SPI NAND 使用 `dd` 的写入命令。

官方参考：[SDK 编译](https://wiki.luckfox.com/zh/Luckfox-Lyra/SDK-Image-Compilation/)、[设备树](https://wiki.luckfox.com/zh/Luckfox-Lyra/Device-Tree/)。

## 上板验收

刷入候选 boot.img 并启动后执行：

```bash
modetest -M rockchip -c
cat /sys/kernel/debug/dri/0/summary
dmesg | grep -Ei 'dsi|panel|touch|ft5|backlight'
cat /proc/bus/input/devices
evtest /dev/input/event0
```

设备号应以 `/proc/bus/input/devices` 为准。预期显示模式为 480×640，触摸 ABS_MT_POSITION_X 范围为 0～479、Y 为 0～639；输入设备名字仍可能包含 `generic ft5x06`，它不是物理芯片 ID。

按以下顺序验证：

1. 冷启动和热重启后均能出图，排除仅继承上次屏幕寄存器状态的情况。
2. 纯红、纯绿、纯蓝及棋盘格无偏色、错位、闪烁、截断。
3. 竖屏四角坐标、拖动、抬起及两触点记录正常。
4. 最后在应用层增加横屏旋转，并同步调整触摸映射，不将面板原生时序改成 640×480。

若有背光但不出图，优先核对 DSI 初始化错误、Lane 0、RGB888 与 `0x3A=0x77` 的一致性，以及冷启动复位行为。若只有冷启动失败，需进一步确认背板 RESET 电路；现有两张图不足以据此指定外部复位 GPIO。
