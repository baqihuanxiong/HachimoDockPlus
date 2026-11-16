# 第三方来源与许可说明

本文件记录 HachimoDockPlus 的主要参考来源、直接依赖及资源许可入口。根目录 [MIT 许可证](LICENSE) 仅授权本项目贡献者有权授权的原创软件和文档，不覆盖下列第三方内容或其派生部分，也不授予商标、账号服务或媒体内容的额外权利。

这里列出的是主要依赖，不是完整的发布合规清单。实际构建使用的间接依赖以 [`src/go.mod`](src/go.mod)、[`src/go.sum`](src/go.sum)、上游包内声明及所分发系统的包清单为准。

## HachimoDock 与米家扩展

- 上游：[YizhengWw/HachimoDock](https://github.com/YizhengWw/HachimoDock)，Copyright (C) 2026 Yizheng Wang。本仓库通过 [`HachimoDock/`](HachimoDock/) 子模块保留参考源码，具体版本由 Git 子模块提交记录固定。
- HachimoDock 自有内容适用其 [项目许可证](HachimoDock/LICENSE)，是**非商业源码可见许可**，并非标准 MIT / GPL 开源许可证。上游对非商业改造、再分发、品牌使用及需另行授权的用途有明确条件；保留致谢不替代授权。
- 上游第三方内容继续适用 [HachimoDock 第三方声明](HachimoDock/THIRD_PARTY_NOTICES.md) 和相应文件的原许可。
- [`src/extensions/mijia/`](src/extensions/mijia/README.md) 的云协议由 HachimoDock 的 `miot_protocol.rs`、`smart_home.rs` 相关逻辑适配，上游涉及 Xiaomi MiLoCo `miloco-miot` 2026.8.6。应同时保留 HachimoDock 的适用许可与 [扩展来源声明](src/extensions/mijia/THIRD_PARTY_NOTICES.md)。
- MiLoCo 相关授权作品为 Copyright (C) 2025 Xiaomi Corporation，适用 [Xiaomi Miloco License](src/extensions/mijia/Xiaomi-Miloco-LICENSE.md)。其非商业及用途限制不会因移植为 Go、独立打包或采用本仓库原创部分而消失。

## 应用依赖与参考实现

| 项目 | 使用方式 / 当前版本 | 许可与原文 |
|---|---|---|
| [PicoClaw](https://github.com/sipeed/picoclaw) | Go 智能体依赖，v0.3.1；另使用其官方图标 | MIT；[随附副本](src/assets/app-icons/PICOCLAW-LICENSE.txt) |
| [LVGL](https://github.com/lvgl/lvgl) | v9.3.0，提交 `c033a98afddd65aaafeebea625382a94020fe4a7`；本仓库保留[本地补丁](src/patches/README.md) | MIT；[随附原文](licenses/LVGL-LICENSE.txt)，被修改的上游部分保留原版权 |
| [Gorilla WebSocket](https://github.com/gorilla/websocket) | v1.5.3，语音 WebSocket 通信 | BSD-2-Clause；[随附原文](licenses/Gorilla-WebSocket-LICENSE.txt) |
| [golang.org/x/net](https://pkg.go.dev/golang.org/x/net) | v0.55.0，网络辅助库 | BSD-3-Clause；[随附原文](licenses/Go-x-net-LICENSE.txt) |
| [NeteaseCloudMusicApiEnhanced](https://github.com/NeteaseCloudMusicApiEnhanced/api-enhanced) | 音乐服务接口定义参考；不打包完整 Node.js 服务 | MIT；[上游原文](https://github.com/NeteaseCloudMusicApiEnhanced/api-enhanced/blob/main/LICENSE) |

Go 工具链、标准库及其余 Go 依赖各自保留原有许可。发布应用二进制时，还需按实际链接的依赖保留版权及许可证；本表不表示仅分发根目录 MIT 文件就已满足全部依赖的要求。

## 字体、图标与语音资源

[`src/tools/bootstrap.sh`](src/tools/bootstrap.sh) 准备字体、图标及参考音频，下载到被 Git 忽略的 `src/.local/assets/`。资源不适用根目录 MIT，构建和分发时应保留各自的许可证与来源说明。

| 资源 | 来源与许可边界 |
|---|---|
| 文泉驿微米黑 | 来自 Debian `fonts-wqy-microhei` 包；字体为 Apache-2.0 或 GPL-3.0-or-later（带字体例外）双重许可，具体组成以包内版权声明为准。引导脚本复制为 `FONT-LICENSE.txt`；分发时还需附所选许可证的完整原文 |
| Noto Emoji | 固定于 Google Fonts 提交 `cc39c4c2c304bc368c9b7c311ea2bcc6013299fe` 的 `ofl/notoemoji`；SIL OFL 1.1。引导脚本保存 `EMOJI-FONT-LICENSE.txt` 与 `EMOJI-FONT-SOURCE.txt` |
| LVGL 内置字体与符号 | 包括 Montserrat；保留 LVGL 及字体目录各自附带的版权、许可和字体声明 |
| PicoClaw 图标 | 原样使用上游 favicon；MIT，见 [图标来源](src/assets/app-icons/README.md) 与 [许可证](src/assets/app-icons/PICOCLAW-LICENSE.txt) |
| NewAPI 图标 | 来自 QuantumNous/new-api；上游仓库提供 AGPL-3.0 原文，已保留于 [NEWAPI-LICENSE.txt](src/assets/app-icons/NEWAPI-LICENSE.txt)。图标固定提交、版权与商标边界见 [图标来源](src/assets/app-icons/README.md)，不转授为 MIT |
| 网易云音乐图标 | 从官方 favicon 转换，用于标识服务；来源与修改记录见 [图标说明](src/assets/app-icons/README.md)。标志及商标归原权利人，未因此取得额外的开放许可 |
| CosyVoice 参考音频 | 引导脚本从 [FunAudioLLM/CosyVoice](https://github.com/FunAudioLLM/CosyVoice/blob/main/asset/zero_shot_prompt.wav) 下载 `zero_shot_prompt.wav`，保存为 `reference.wav` 并记录 `VOICE-SOURCE.txt`。仓库代码许可不等同于声音素材和声音使用权的统一授权；对外分发前应核对该素材的具体授权，或替换为自有 / 明确获授权的录音 |
| 其他测试音频 | 如 `sandbox/audio/` 下的音乐文件，不适用本仓库 MIT；发布包不应将其视为自由再分发的测试资源 |

## 系统、驱动与构建工具

- **Luckfox / Rockchip SDK、Linux 与 Buildroot**：用于系统、内核及驱动构建。SDK 内不同组件采用不同许可；Linux 内核整体采用 GPL-2.0-only，具体文件、设备树与绑定仍以其 SPDX 和原始声明为准。`board/` 中源于 SDK / 内核的适配部分保留其适用许可。本机 `sdk/` 不纳入 Git，根目录 MIT 不覆盖 SDK 或固件镜像。
- **ALSA、FFmpeg、BlueZ 与 BlueALSA**：应用通过板端工具或系统服务实现录放音、解码、蓝牙管理与音频路由。它们并非统一 MIT 授权；FFmpeg 的具体许可还取决于构建选项。分发固件或这些工具时，须保留实际版本的许可，并履行适用的源码提供等义务。
- **Samba 与其共享库**：作为独立进程提供可选 SMB 配置共享。Samba 主要采用 GPL-3.0-or-later，所带库可能使用其他许可；[`build-samba.sh`](src/tools/build-samba.sh) 与 [`collect_samba.py`](src/tools/collect_samba.py) 会将 Debian 包版权文件、版本和来源入口打包到运行时归档。分发前仍需核对完整许可证及对应版本源码的提供方式。
- **Go、Debian、Docker 与交叉工具链**：用于开发和构建；各组件按自己的许可提供，构建环境的使用不改变本仓库各部分的授权范围。

## 硬件与厂商资料

[`hardware/`](hardware/) 包含个人定制底板资料、器件数据手册、厂商原理图及引脚参考。**目前没有为硬件设计统一选择开放硬件许可证**，根目录 MIT 也不覆盖这些资料。器件手册、参考电路与第三方设计归对应权利人；公开可访问不等于可任意改编、再许可或商业分发。

嘉立创、嘉立创 EDA、幸狐与 Rockchip 等平台的名称用于来源说明和致谢，不表示商标授权、合作或官方背书；免费打样属于平台服务，条件以平台公布为准。
