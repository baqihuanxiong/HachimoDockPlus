## 项目简介

基于 [HachimoDock](https://github.com/YizhengWw/HachimoDock) 项目进行软硬件个人定制的 Linux 桌面交互终端。

将原方案的 ESP32-P4 主控替换为 **幸狐 Luckfox Lyra Zero W / RK3506B**，以 **Go + LVGL** 构建应用，探索在一台带触屏、按键、摇杆和音频的小设备上运行 AI 对话、音乐与日常工具。本仓库整理定制底板资料、板级适配、应用源码和实机验证记录，供学习、改造与复刻参考。

> **开发中，尚未达到完整复刻交付状态。**
>
> - **SPDIF（S/PDIF）和蓝牙音频尚未完成适配**；已有配置项或界面入口不代表链路可用，蓝牙设备管理也不等于蓝牙音频可用。
> - **大部分 HachimoDock 官方功能尚未迁移**，包括官方桌面客户端协同、Agent 状态跟随、宠物形象及组件生态等；当前版本不承诺兼容官方固件、客户端或组件包。
> - 当前验证主要覆盖下述 Lyra Zero W 硬件组合，硬件资料与复刻流程仍在完善；采购、打板和烧录前请核对资料版本及已知限制。

[当前进度](#当前进度) · [复刻与开发](#复刻与开发) · [硬件说明与组装](hardware/OSHWHub-project.md) · [项目结构](#项目结构) · [许可证](#许可证) · [致谢](#致谢)

## 项目定位

HachimoDock 提供了桌面交互终端的创意与实现，本项目在此基础上更换主控、定制底板，并面向 Linux 重新组织应用与硬件服务。这是个人维护的改造版本，与官方版本有不同的硬件、软件架构和开发进度。

当前方案以板端应用运行，Go 负责业务、网络和服务编排，LVGL 负责图形界面；AI、语音和部分在线组件仍需要自行配置外部服务。可复用的板级适配、应用接口与验证过程分别保存在对应目录中，便于在此基础上继续开发。

## 当前进度

以下状态针对当前硬件基线；“已验证”不代表所有使用场景均已覆盖。

| 模块 | 状态与范围 | 说明 |
|---|---|---|
| 显示与触摸 | 已完成实机验证 | WLK2802MIPI / ST7701S 屏幕、CST826 触摸；原生 480×640，应用横屏 640×480 |
| USB 音频与扬声器 | 已完成实机验证 | CM108B 录放音、PAM8403 功放控制与全局音量；见 [USB 音频说明](board/lyra-zero-w/usb-audio.md) |
| 按键与摇杆 | 已接入主程序 | SW1/SW2/SW3 和 ADS1015 两轴输入；摇杆按下尚未绑定公共动作 |
| Wi-Fi / 蓝牙管理 | 已实现基础功能 | 网络配置、蓝牙扫描与配对等；具体限制见 [设置说明](src/references/settings.md) |
| PicoClaw 对话 | 已实现 | 文字、按键语音、ASR / TTS，以及技能、CLI 和 MCP 扩展；需配置服务 |
| 网易云音乐 | 已实现 | 短信登录、歌单、封面、歌词与播放控制；受账号、服务及内容权限限制 |
| NewAPI 监控 | 已实现 | 用量看板、上游余额与用户余额重置；需配置自己的实例 |
| 设置与系统服务 | 已实现 | 主题、时钟 / 纯黑屏保、开机自启、配置共享；见 [应用说明](src/README.md) |
| 电池状态 | 已接入，精度待完善 | INA226 电压、电流采集；电量仅为电压粗估，尚未完成电芯标定 |
| **SPDIF / S/PDIF** | **待适配** | 已有软件输出选择，尚未完成硬件链路适配与实机验收 |
| **蓝牙音频** | **待适配** | 已有 BlueALSA 路由代码；实际发声及 Wi-Fi 共存仍未验收，见 [板级无线说明](board/lyra-zero-w/wifi.md) |
| **官方功能迁移** | **大部分未迁移** | 本表仅描述本改造版能力；官方功能以 [HachimoDock 文档](HachimoDock/README.md) 为参考 |

后续重点：完成两条音频输出链路的适配与验收，逐步评估官方功能迁移，并补齐硬件生产资料、装配说明及完整复刻记录。

## 复刻与开发

### 1. 获取源码与上游参考

```sh
git clone --recurse-submodules https://github.com/baqihuanxiong/HachimoDockPlus.git
cd HachimoDockPlus

# 已克隆仓库但未初始化上游参考时执行
git submodule update --init --recursive
```

`HachimoDock/` 是固定到指定提交的上游参考子模块；当前 Linux 应用源码位于 `src/`。请按本仓库文档构建本改造版。

### 2. 核对硬件与系统基线

当前采用 Lyra Zero W / RK3506B、SPI NAND、Buildroot、Linux 6.1.99，SDK 基线为 `Luckfox_Lyra_SDK_202610`，应用使用 ARMv7 hard-float / glibc 工具链。

- 硬件说明与组装：[立创开源硬件平台项目描述](hardware/OSHWHub-project.md) 汇总硬件参数、电路原理、装配流程与注意事项。
- 底板资料：[原理图](hardware/SCH_底板_2026-11-07.pdf)、[PCB 图](hardware/PCB_底板_2026-11-07.pdf)、[BOM](hardware/BOM_底板_2026-11-07.xlsx) 与 [引脚表](hardware/Pinout.md)。目前仓库主要提供 PDF、BOM 和参考资料，**尚未提供完整的可编辑 PCB 工程与 Gerber 生产文件**。
- 板级准备：[Lyra Zero W 适配说明](board/lyra-zero-w/README.md) 与 [幸狐官方文档](https://wiki.luckfox.com/zh/Luckfox-Lyra/Luckfox-Lyra-Zero-W/)。本机 SDK、分区备份与固件产物不随 Git 分发，需要自行准备。
- 接线核对：[硬件适配基线](src/references/hardware-adaptation.md) 汇总当前已验证的显示、输入、音频和电池接口。**当前屏幕使用 MIPI DSI；引脚表中的历史 SPI 屏分配不适用于此连接。**

### 3. 构建应用并配置服务

开发主机需要 Docker、Python 3 和网络。完成硬件基线核对后，可先构建应用：

```sh
cd src
tools/dev.sh tools/bootstrap.sh
tools/dev.sh tools/build.sh native
tools/dev.sh tools/build.sh arm
```

本机构建用于预览和诊断，ARM 构建用于板端部署。配置文件、资源路径、离屏预览、部署与开机自启步骤见 [应用构建与部署](src/README.md)。从 [`config.example.json`](src/config.example.json) 创建自己的本地配置；服务地址、模型及密钥需自行填写，示例值不能直接用于服务调用。

凭据、账号数据和私有配置应保存在被 Git 忽略的本地目录中。不要将其写入公开文档、问题反馈或提交记录。

### 4. 按组件验证与扩展

从 [开发参考索引](src/references/README.md) 开始，按 [应用开发规范](src/references/app-development.md) 扩展组件；按 [实机闭环测试](src/references/board-validation.md) 记录硬件、固件基线、结果与未验证项。编译通过、离屏预览和旧试验记录不能替代当前硬件验收。

## 项目结构

| 路径 | 内容 |
|---|---|
| [board/lyra-zero-w](board/lyra-zero-w/README.md) | 设备树、屏幕与触摸、网络、USB 音频及固件工具 |
| [src](src/README.md) | Go + LVGL 应用、平台服务、测试、构建与部署 |
| [src/references](src/references/README.md) | 组件文档、开发契约、硬件基线与测试报告 |
| [hardware](hardware/) | 定制底板原理图、PCB 图、BOM、引脚定义及器件资料 |
| [sandbox](sandbox/README.md) | 按功能整理的开发试验、验证脚本与排查记录 |
| [HachimoDock](HachimoDock/README.md) | 官方项目参考子模块，保留上游来源与许可 |
| `sdk/` | 本机 SDK、固件备份和构建产物，不纳入 Git |
| [licenses](licenses/) / [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) | 第三方许可证副本、来源和适用范围 |

后续支持其他开发板时，在 `board/<开发板名>/` 下添加板级适配；功能试验放在 `sandbox/<功能名>/` 下，并注明开发板与固件基线。

## 参与改进

欢迎通过 [Issues](https://github.com/baqihuanxiong/HachimoDockPlus/issues) 提交复刻反馈、问题和功能建议，也欢迎提交文档、驱动适配与组件改进。硬件相关反馈请附开发板、底板 / 接线版本、固件基线、复现步骤和脱敏日志；适配贡献请说明实测范围与尚未验证的部分。

## 许可证

本项目**原创的软件和文档采用 [MIT License](LICENSE)**，方便学习、修改和复用。该授权仅覆盖贡献者有权授权的原创部分；第三方代码、派生实现、硬件设计、字体、图片、音频及商标不因仓库根目录的 MIT 许可证而改变权利归属或许可。

- **HachimoDock 上游内容及其派生部分**保留 [HachimoDock 项目许可证](HachimoDock/LICENSE) 与 [上游第三方声明](HachimoDock/THIRD_PARTY_NOTICES.md)。其自有内容采用非商业源码可见许可，不能将整个仓库描述为统一 MIT 授权，也不能据此推定商业使用已获许可。
- **米家扩展**涉及 HachimoDock 与 Xiaomi MiLoCo 的适配，保留其 [来源声明](src/extensions/mijia/THIRD_PARTY_NOTICES.md) 和 [Xiaomi Miloco License](src/extensions/mijia/Xiaomi-Miloco-LICENSE.md)；根目录 MIT 不替代这些授权条件。
- 其余依赖与素材见 [第三方来源与许可说明](THIRD_PARTY_NOTICES.md)。硬件设计与厂商参考资料暂未统一授予额外许可，具体使用与分发须核对对应权利人的授权。

## 致谢

首先感谢 **[HachimoDock 官方开发者 Yizheng Wang（@YizhengWw）](https://github.com/YizhengWw)** 及所有贡献者，提供桌面交互终端的创意、软硬件参考与开发资料，让本次个人定制有了起点。

也感谢以下平台、团队与开源项目：

| 平台 / 项目 | 对本项目的帮助 |
|---|---|
| [嘉立创](https://www.jlc.com/) / [嘉立创 EDA](https://pro.lceda.cn/) | PCB 设计与制造平台，感谢**嘉立创免费打样**服务降低个人硬件试制与迭代门槛 |
| [幸狐 Luckfox](https://wiki.luckfox.com/zh/Luckfox-Lyra/Luckfox-Lyra-Zero-W/) / [Rockchip](https://www.rock-chips.com/) | Lyra Zero W 开发板、RK3506B 平台、SDK 与硬件文档 |
| [Linux](https://www.kernel.org/) / [Buildroot](https://buildroot.org/) | 嵌入式系统、驱动及根文件系统基础 |
| [Go](https://go.dev/) / [LVGL](https://github.com/lvgl/lvgl) | 应用开发语言、运行时与嵌入式图形界面 |
| [Sipeed PicoClaw](https://github.com/sipeed/picoclaw) | 板端智能体、工具调用与扩展能力 |
| [Xiaomi MiLoCo](https://github.com/XiaoMi/xiaomi-miloco) / [CosyVoice](https://github.com/FunAudioLLM/CosyVoice) | 可选米家扩展的协议参考与语音试验参考资源 |
| [ALSA](https://www.alsa-project.org/) / [FFmpeg](https://ffmpeg.org/) | 音频采集、播放与音乐解码 |
| [BlueZ](https://www.bluez.org/) / [BlueALSA](https://github.com/arkq/bluez-alsa) / [Samba](https://www.samba.org/) | 蓝牙管理、待完善的蓝牙音频链路及局域网配置共享 |
| [Gorilla WebSocket](https://github.com/gorilla/websocket) | 语音服务的 WebSocket 通信 |
| [NeteaseCloudMusicApiEnhanced](https://github.com/NeteaseCloudMusicApiEnhanced/api-enhanced) / [NewAPI](https://github.com/QuantumNous/new-api) | 音乐接口参考、API 服务管理与监控对接 |
| [文泉驿](https://wenq.org/) / [Noto Emoji](https://github.com/googlefonts/noto-emoji) / [Debian](https://www.debian.org/) / [Docker](https://www.docker.com/) | 中文与 Emoji 字体、构建工具链和可复用的开发环境 |

以上名称仅用于说明来源与致谢，不表示相关平台或开发者对本改造版本提供官方授权、合作或背书。
