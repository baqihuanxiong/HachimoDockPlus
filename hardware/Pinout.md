# 底板引脚分配

更新日期：2026-11-07。以[当前原理图](SCH_底板_2026-11-07.pdf)为准，已加入 INA226；屏幕仍需配置设备树和驱动，尚未实机验证。

## 板载资源核对

| 资源 | 实际占用 | 与本方案的关系 |
|---|---|---|
| Wi-Fi＋蓝牙 | AIC8800DC 经 CH334F 的 USB 下行端口1；GPIO1_C5 电源使能、GPIO1_C7 天线选择 | 不占屏幕 SPI0 / I²C0，也不占调试 UART0；两个控制脚均未引到 PI1 |
| NAND Flash | FSPI：GPIO2_A0=CS、A1=CLK、A2～A5=D0～D3 | 独立于 SPI0；不在 PI1 上 |
| TF 卡 | GPIO3_A0～A5，GPIO1_B6 检测、GPIO1_B7 电源控制 | 均不在 PI1 上；与 Flash 可并存 |
| MIPI DSI | 专用 DPHY；GPIO1_C4、GPIO0_B7 控制；I²C2=GPIO0_A0/A1 | SDK 屏幕节点地址 0x14/0x38/0x45/0x5d，与 ADS1015=0x48、INA226=0x40、IP5306=0x75 不冲突；换屏时复核 |
| 板载 LED／温控 | GPIO1_A0／GPIO0_C5；默认 PWM0_CH2 在 GPIO0_B7 | 均不占本方案 GPIO；SPI 屏用独立 PWM1_CH0 |
| CM108B 音频 | 底板 U6 接核心板 H5，USB 下行端口3 | 与无线模块分处 HUB 端口，可同时枚举；共享 USB 带宽 |

## 40Pin 分配

PI1 为核心板物理针号，U2 为底板背面安装排母针号，奇偶成对互换。

| PI1 | U2 | GPIO / RM_IO | 用途 | 屏幕 FPC |
|---:|---:|---|---|---:|
| 1 | 2 | — | 开发板3.3V，底板此脚NC | — |
| 2 | 1 | — | +5V，底板向核心板供电 | 14 |
| 3 | 4 | GPIO0_A0 / 0 | I2C_SDA；I2C2_SDA；IP5306 / ADS1015 / INA226 | — |
| 4 | 3 | — | +5V，底板向核心板供电 | 14 |
| 5 | 6 | GPIO0_A1 / 1 | I2C_SCK；I2C2_SCL；IP5306 / ADS1015 / INA226 | — |
| 6 | 5 | — | 公共地 | — |
| 7 | 8 | GPIO0_A2 / 2 | SW1_SIG；SW1，输入上拉、按下接地 | — |
| 8 | 7 | GPIO0_C6 / 22 | TXD；UART0_TX；调试输出 | — |
| 9 | 10 | — | 公共地 | — |
| 10 | 9 | GPIO0_C7 / 23 | RXD；UART0_RX；调试输入 | — |
| 11 | 12 | GPIO0_A3 / 3 | SW2_SIG；SW2，输入上拉、按下接地 | — |
| 12 | 11 | GPIO0_B6 / 14 | NC；空闲 | — |
| 13 | 14 | GPIO0_A4 / 4 | SW3_SIG；SW3，输入上拉、按下接地 | — |
| 14 | 13 | — | 公共地 | — |
| 15 | 16 | GPIO0_A5 / 5 | JOYSTICK_KEY；摇杆按键，输入上拉、按下接地 | — |
| 16 | 15 | GPIO0_B5 / 13 | SPDIF_EN；GPIO 输出；SPDIF 缓冲使能 | — |
| 17 | 18 | — | 3V3 经 R21（0Ω）向屏幕供电 | 13 |
| 18 | 17 | GPIO0_B4 / 12 | SPK_EN；GPIO 输出；功放使能，默认低 | — |
| 19 | 20 | GPIO0_A6 / 6 | LCD_MOSI；SPI0_MOSI | 10 |
| 20 | 19 | — | 公共地 | — |
| 21 | 22 | GPIO0_A7 / 7 | NC；空闲 | — |
| 22 | 21 | GPIO0_B3 / 11 | LCD_DC；GPIO 输出；高=数据，低=命令 | 8 |
| 23 | 24 | GPIO0_B0 / 8 | LCD_SCLK；SPI0_CLK | 11 |
| 24 | 23 | GPIO0_B1 / 9 | LCD_CS_N；SPI0_CSN0，低有效 | 9 |
| 25 | 26 | — | 公共地 | — |
| 26 | 25 | GPIO0_B2 / 10 | NC；空闲 | — |
| 27 | 28 | GPIO1_B1 / 24 | TP_SDA；I2C0_SDA | 4 |
| 28 | 27 | GPIO1_D3 / 31 | TP_SCL；I2C0_SCL | 5 |
| 29 | 30 | GPIO1_B2 / 25 | LCD_RST_N；GPIO 输出；LCD 独立复位，低有效 | 7 |
| 30 | 29 | — | 公共地 | — |
| 31 | 32 | GPIO1_B3 / 26 | LCD_BL_PWM；PWM1_CH0；背光控制，默认低 | 6 |
| 32 | 31 | GPIO1_D2 / 30 | TP_INT_N；GPIO 中断输入；有效方式按组件/驱动确认 | 2 |
| 33 | 34 | GPIO1_C2 / 27 | TP_RST_N；GPIO 输出；FT6336U 独立复位，低有效 | 3 |
| 34 | 33 | — | 公共地 | — |
| 35 | 36 | GPIO0_C0 / 16 | NC；空闲 | — |
| 36 | 35 | GPIO1_D1 / 29 | NC；空闲 | — |
| 37 | 38 | GPIO1_C3 / 28 | NC；空闲 | — |
| 38 | 37 | GPIO0_C1 / 17 | NC；空闲 | — |
| 39 | 40 | — | 公共地 | — |
| 40 | 39 | GPIO0_C2 / 18 | NC；空闲 | — |

## 屏幕 FPC：X1，14Pin 0.5mm

所有信号为 3.3V 逻辑。J1 指鸿讯 28005 模块接口；下表是信号转接关系，X1 与 J1 不能同号直连。X1 的 15、16 为固定焊脚，原理图标为 NC。

| FPC | 网络 | PI1 | U2 | 28005 J1 |
|---:|---|---:|---:|---:|
| 1 | GND | — | — | 2 |
| 2 | TP_INT_N | 32 | 31 | 14 |
| 3 | TP_RST_N | 33 | 34 | 11 |
| 4 | TP_SDA | 27 | 28 | 12 |
| 5 | TP_SCL | 28 | 27 | 10 |
| 6 | LCD_BL_PWM | 31 | 32 | 8 |
| 7 | LCD_RST_N | 29 | 30 | 4 |
| 8 | LCD_DC | 22 | 21 | 5 |
| 9 | LCD_CS_N | 24 | 23 | 3 |
| 10 | LCD_MOSI | 19 | 20 | 6 |
| 11 | LCD_SCLK | 23 | 24 | 7 |
| 12 | GND | — | — | 2 |
| 13 | 3V3（经 R21） | 17 | 18 | 1 |
| 14 | +5V | 2、4 | 1、3 | 不接 |

- pin 13 的 3.3V 由核心板 PI1-17（底板 U2-18）经 R21（0Ω）直供。pin 14 的 +5V 为定制屏幕底板预留，不能直接接鸿讯模块 VCC 或裸屏背光；若用 +5V 稳压得到屏幕 3.3V，拆除 R21，避免电源并联。
- pin 1、12 均接公共地；屏幕侧保留去耦。背光驱动／限流在屏幕侧，pin 6 只作控制，软件配置默认低。
- LCD 不接 MISO，模块 J1-9、13 悬空；驱动按固定 LCD 型号只写初始化。FT6336U 走独立 I²C0，LCD 与触摸独立复位；触摸上拉至屏幕 3.3V。

## SDK 配置约束

- 使用 `luckfox_lyra_zero-w_buildroot_spinand_defconfig`，对应 `rk3506b-luckfox-lyra-zero-w.dts`；保留 FSPI、USB Host、无线电源 GPIO 和 UART0 调试功能。
- SPI0 的 `pinctrl-0` **替换为** `rm_io8_spi0_clk`、`rm_io6_spi0_mosi`、`rm_io9_spi0_csn0`，不要沿用默认原生引脚组；MISO 和第二片选不启用，同一片选不能再挂 spidev。
- I²C0 启用并选择 `rm_io31_i2c0_scl`、`rm_io24_i2c0_sda`；I²C2 保留现有 `rm_io1_i2c2_scl`、`rm_io0_i2c2_sda`，供电管理与 ADC 共用该总线。
- 背光用 `pwm1_8ch_0`，活动状态只选 `rm_io26_pwm1_ch0`；不得通过动态 `config` 状态将 PWM 广播到其他 RM_IO。按键配置输入上拉；音频使能和背光默认低。

## 电池电压／电流检测：U10 INA226

I²C2 地址为 **0x40**；SDA-4、SCL-5 分别接底板 U2-4、U2-6，VS-6 接 3V3。A1-1、A0-2、GND-7 接地，ALERT-3 不用时悬空。

R20=10mΩ 串在 U9 与系统 VBAT 之间；IN+-10 接 VBAT_IN+（电池侧），IN−-9 接 VBAT_IN−（系统侧），放电为正、充电为负。VBUS-8 经 R19=100Ω 读取系统侧 VBAT，C38=100nF 滤波，C37=100nF 为供电去耦。ADS1015 的 AIN2/3 均接地，仅 AIN0/1 测摇杆。
