# 应用组件开发参考

本目录是后续应用组件开发的统一入口。基线整理于 **2026-11-15**，适用于当前 Lyra Zero W / RK3506B、Go + LVGL 单进程应用框架。

## 阅读顺序

1. [硬件适配基线](hardware-adaptation.md)：设备路径、引脚、共享资源、可复用接口和已验证限制。
2. [应用组件开发规范](app-development.md)：依赖边界、生命周期、配置、页面、主题、性能及交付流程。
3. [实机闭环测试](board-validation.md)：构建、部署、输入、截图、实际副作用、资源释放和复测。
4. [SMB 配置共享](configuration-sharing.md)：远程编辑低频配置、共享账号、校验与程序重启。
5. [实机测试报告模板](templates/board-test-report.md)：每次交付填写版本、步骤、预期、证据及结论。

当前未完成的板端验证见 [蓝牙实机验证交接](bluetooth-handoff.md)，记录串口迁移时的设备状态、待部署修复及恢复步骤。

开发一个组件时，先确定硬件与验收用例，再实现业务和页面，最后执行实机闭环；目录能打开或服务进程存活都不能单独证明功能完成。

## 文档与代码分工

| 位置 | 内容 |
|---|---|
| 本目录 | 当前应用开发约定、硬件使用方式和测试流程 |
| [`internal/core`](../internal/core/) | 应用接口、事件和生命周期的实际契约 |
| [`internal/bootstrap/apps.go`](../internal/bootstrap/apps.go) | 应用工厂、共享服务、页面和终端命令的装配入口 |
| [`internal/platform`](../internal/platform/) | 可复用的硬件、网络及音频能力 |
| [`board/lyra-zero-w`](../../board/lyra-zero-w/README.md) | 设备树、固件、内核模块及板级适配原始记录 |
| [`hardware`](../../hardware/Pinout.md) | 原理图与物理引脚，部分屏幕设计属于历史方案 |
| [`sandbox`](../../sandbox/README.md) | 独立硬件试验、测量脚本和历史验证记录 |
| [`patches`](../patches/README.md) | 固定版本 LVGL 的本地补丁及维护说明 |
| [`examples/board_ui_probe.py`](examples/board_ui_probe.py) | 主机端截图、触摸点击和滑动示例，不安装到应用中 |

发现文档与实现不一致时，核对代码、当前设备枚举和实测证据后更新相应文档；不要把旧试验参数直接当作当前应用默认值。修改板型、固件或设备后，重新确认本目录的硬件基线。

## 既有协议参考

- [FunASR 接口](asr_interface.md)
- [CosyVoice 接口](tts_interface.md)
- [网易云音乐实现与验证](music.md)
- [PicoClaw 通用工具、技能、CLI 与 MCP](picoclaw-extensions.md)
- [独立米家 CLI](../extensions/mijia/README.md)

服务协议与实际端点的可用性分别验证。不能把 HTTP 成功、流连接建立或返回了部分内容，等同于完整文本、完整音频和最终播放均成功。

`references/secret.md` 是本地、被 Git 忽略的配置导入来源，不属于可公开的开发规范。文档、示例和测试报告不得复制其中的凭据，也不要提交 `.local/`、`build/`、`.cache/` 或包含对话、账号信息的原始采集文件。
