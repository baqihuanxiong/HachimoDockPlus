## 项目简介

基于 [HachimoDock](https://github.com/YizhengWw/HachimoDock) 的创意与参考实现进行个人定制，将原方案的 ESP32-P4 主控替换为幸狐 **Luckfox Lyra Zero W / RK3506B**，重新设计配套底板，并使用 **Go + LVGL** 构建 Linux 应用。

项目将触摸屏、麦克风、双声道扬声器、机械按键、摇杆和电池监测组合为一台桌面终端，探索 AI 对话、音乐播放及日常工具的使用方式，也为嵌入式 Linux、USB 音频和软硬件协同开发提供参考。

![MyHachimoDock_Opened.jpg](https://image.lceda.cn/oshwhub/pullImage/e02e67fac9d2493bb0f099f6c3163941.jpg)

> **当前硬件仍在测试中，已知以下问题尚未解决：**
>
> - **SPDIF（S/PDIF）音频输出尚未调通。**
> - **移除 Type-C 外部供电、转为电池供电时，会出现瞬间断电，供电切换缺陷尚未解决。**

本页侧重硬件参数、电路原理与组装流程；项目整体进度、软件构建与开发入口见 [项目 README](https://github.com/baqihuanxiong/HachimoDockPlus/blob/master/README.md)。

## 项目功能

| 功能 | 当前实现与边界 |
|---|---|
| 触屏交互 | WLK2802MIPI 屏幕与 CST826 触摸已完成实机验证；应用采用 640×480 横屏界面 |
| AI 对话 | 接入 PicoClaw，支持文字、按键录音、语音识别与语音播报，可扩展技能、CLI 和 MCP；需自行配置模型与语音服务 |
| 音乐播放 | 支持网易云音乐短信登录、歌单、封面、歌词与播放控制；可用内容取决于账号及服务权限 |
| USB 录放音 | CM108B USB 声卡配合麦克风和 PAM8403 功放，已验证录音、双声道播放及应用所需的采样率转换 |
| 按键与摇杆 | 三个独立按键、两轴摇杆用于目录选择、返回及应用操作；摇杆按下已接线，尚未绑定公共动作 |
| 电池监测 | INA226 采集电压与双向电流，主程序显示电池状态；电量为电压粗估，尚未完成电芯标定 |
| 系统与工具 | 提供 Wi-Fi 配置、基础蓝牙设备管理、主题、时钟 / 纯黑屏保、配置共享和 NewAPI 监控 |
| SPDIF / 蓝牙音频 | 预留硬件或软件入口，**仍待适配及实机验收**；能够扫描或配对蓝牙设备不代表音频链路可用 |

项目适合作为个人桌面工具、语音交互终端，以及嵌入式 Linux 图形界面和外设开发的学习平台。现阶段尚需根据文档完成固件准备、应用构建和服务配置。

![Screenshot_Overview.png](https://image.lceda.cn/oshwhub/pullImage/2991895663ea45efafa2c38242da0be7.png)

## 项目参数

以下为当前开发样机及仓库文档采用的基线；更换核心板、屏幕、器件或固件后需重新验证。

| 项目 | 参数 / 方案 |
|---|---|
| 开发板 | Luckfox Lyra Zero W |
| 主控 | Rockchip RK3506B，应用目标为 ARMv7 Linux |
| 系统 | Buildroot、Linux 6.1.99、SPI NAND；SDK 基线 `Luckfox_Lyra_SDK_202610` |
| 应用 | Go 1.25.11 + LVGL v9.3.0，ARM hard-float / glibc |
| 显示 | WLK2802MIPI / ST7701S，单 Lane MIPI DSI、RGB888 |
| 分辨率与触摸 | 屏幕原生 480×640，应用横屏 640×480；CST826 触摸 |
| 外部供电 | USB Type-C，使用 5 V 供电 |
| 电池电源 | IP5306 充放电电路，配合单节锂电池；容量按实际选型确定 |
| 音频 | CM108B USB Audio；PAM8403 双声道功放；麦克风输入 |
| 音频格式 | CM108B 原生 44.1 / 48 kHz；已验证经 ALSA 转换的 16 kHz 单声道录音和 24 kHz 单声道播放 |
| 扬声器 | 原理图标注 8 Ω / 2 W，属于扬声器额定规格，不代表整机实测输出功率 |
| 操作输入 | SW1 / SW2 / SW3、两轴摇杆及摇杆按键；ADS1015 采集两轴 |
| 电池采集 | INA226，10 mΩ 采样电阻；I²C 地址 `0x40` |
| I²C 外设 | ADS1015：`0x48`；INA226：`0x40`；IP5306：`0x75`，当前未实现其应用层管理服务 |
| 无线与调试 | 开发板提供 Wi-Fi、蓝牙；底板引出 3.3 V TTL 调试串口 |
| 数字音频预留 | 3.5 mm 同轴 S/PDIF 输出电路，适配未完成 |

当前底板参考资料版本为 **2026-11-07**。板框、安装孔、封装和生产参数以平台实际上传的 PCB 工程与生产文件为准。

## 物料清单

| 类别 | 物料 | 数量 | 单价 | 备注 |
|---|---|---|---|---|
| 开发板 | [幸狐 Lyra Zero W](https://detail.tmall.com/item.htm?id=939604128887) | 1 块 | ￥185 | 淘宝购买 |
| 底板 | PCB | 1 片 | ￥0 | 嘉立创免费打样 |
| 元器件 | BOM | 1 套 | ￥170 | 价格来自立创商城，PAM8403DR-H、CM108B、ADS1015IDGSR、IP5306 芯片可以在淘宝买省几十块 |
| 钢网 | 钢网 | 1 张 | ￥20 | 随嘉立创 PCB 一起下单 |
| 外壳 | [改版外壳](https://github.com/baqihuanxiong/HachimoDockPlus/tree/master/hardware/models/) | 1 套 | ￥0 | 可自行打印或找代工厂打印，模型用 codex 接 blender 改过，需打印[改版底壳](https://github.com/baqihuanxiong/HachimoDockPlus/tree/master/hardware/models/lower-modified.stl)，其他可以用官方原版模型 |
| 屏幕 | [2.8 英寸 480×640 IPS MIPI-DSI 显示屏](https://item.taobao.com/item.htm?id=674784446079) | 1 套 | ￥85 | 这个目前买不到，但社区有其他大佬魔改的屏幕 |
| 电池 | [18650锂电池 3.7V 3400maH PH 接口](https://detail.tmall.com/item.htm?id=795700071122) | 1 个 | ￥28.9 | 淘宝购买 |
| 扬声器 | [腔体喇叭 2030-8Ω2W-1.25](https://item.taobao.com/item.htm?id=986847812412) | 2 个 | ￥6.3 | 淘宝购买 |
| 线缆 | [MX1.25-4P 双头反向 100mm](https://item.taobao.com/item.htm?id=1081587022695) | 1 根 | ￥3.78 | 淘宝购买 |
| 按键 | [凯华 Kailh BOX 机械键盘轴](https://detail.tmall.com/item.htm?id=639271554533) | 3 个 | ￥10 | 淘宝购买 |
| 键帽 | 任意标准键帽 | 3 个 | ￥0 | 可自行购买或 3D 打印，自行购买需要圆角键帽，方的会和底壳干涉 |
| 摇杆 | [YA13S-L7.4-B10Ka(60)-0-DL01 摇杆电位器](https://item.szlcsc.com/39051755.html) | 1 个 | ￥3.54 | 价格来自立创商城，立创没有现货也可以淘宝买 |
| 螺丝 | [304 不锈钢十字圆头螺钉 M2.5×6](https://detail.tmall.com/item.htm?id=637524754721) | 2 颗 | ￥3.36 | 淘宝购买，没买热熔螺母，螺丝直接拧也挺牢的 |
| 组装 | 焊接与调试 | 1 次 | ￥0 | 自行焊接，需要准备恒温加热台、钢网夹具、锡膏、焊油、电烙铁、万用表 |

## 软件代码

项目源码与持续更新的开发文档：**[GitHub 仓库](https://github.com/baqihuanxiong/HachimoDockPlus)**。

## 来源与许可

本项目为 HachimoDock 的个人改造版本。原创软件和文档按仓库 [MIT 许可证](https://github.com/baqihuanxiong/HachimoDockPlus/blob/master/LICENSE) 提供；硬件设计不自动适用该 MIT 许可证，其授权以平台发布页明确标注的许可及相关权利人的授权为准。

HachimoDock 上游内容及派生部分、米家扩展和其他第三方内容保留各自许可，不因在本平台发布而改变。具体来源与适用范围见 [第三方许可说明](https://github.com/baqihuanxiong/HachimoDockPlus/blob/master/THIRD_PARTY_NOTICES.md)，不能将全部内容视为统一 MIT 授权。

## 致谢

感谢 **[HachimoDock 官方开发者 Yizheng Wang（@YizhengWw）](https://github.com/YizhengWw)** 及贡献者提供桌面终端的创意、软硬件实现与开发资料。

感谢 **嘉立创免费打样**、嘉立创 EDA 与立创开源硬件平台，降低个人硬件设计、试制和分享的门槛；感谢幸狐 Luckfox、Rockchip 提供核心板、芯片平台与 SDK 资料。

同时感谢 Linux、Buildroot、Go、LVGL、Sipeed PicoClaw、ALSA、FFmpeg、BlueZ、BlueALSA、Samba、Gorilla WebSocket、文泉驿、Noto Emoji、NeteaseCloudMusicApiEnhanced、NewAPI、Xiaomi MiLoCo、CosyVoice，以及 Debian、Docker 等项目和社区提供的基础能力与参考资源。
