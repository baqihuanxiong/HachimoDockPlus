# 开发参考索引

本目录维护 Go + LVGL 应用层的开发约定、组件用法与验证方法。项目概览、构建和部署命令见 [src/README.md](../README.md)；当前硬件基线为 Lyra Zero W / RK3506B。

## 按任务阅读

| 任务 | 参考 |
|---|---|
| 新增或修改应用组件 | [应用开发规范](app-development.md)：依赖边界、生命周期、配置、页面、主题和性能 |
| 使用硬件或更换开发板 | [硬件适配基线](hardware-adaptation.md)：设备、引脚、共享资源和已知限制 |
| 验证与交付 | [实机闭环测试](board-validation.md)、[报告模板](templates/board-test-report.md) |
| 编辑配置、密钥与资源 | [SMB 配置共享](configuration-sharing.md)：安装、原子应用、音频输出和校验 |
| 截图或注入触摸事件 | [主机端示例](examples/board_ui_probe.py)，使用方式与验证范围见[测试流程](board-validation.md#页面与触摸) |

新增组件建议依次确认硬件与验收用例、实现应用及页面、执行实机闭环。依赖、接口与默认值以当前代码为准，文档和代码不一致时核对实现及实测证据后更新；更换板型或固件后重新确认硬件基线。

## 组件与协议

| 主题 | 参考 |
|---|---|
| 组件中心、外观与无线设置 | [设置与公共界面](settings.md) |
| PicoClaw 对话 | [使用与配置](picoclaw.md)、[工具 / 技能 / CLI / MCP](picoclaw-extensions.md)、[可选米家 CLI](../extensions/mijia/README.md) |
| 语音服务 | [FunASR 协议](asr_interface.md)、[CosyVoice 协议](tts_interface.md)；这些是既有服务部署的接口记录，可用性需另行验证 |
| 网易云音乐 | [操作、实现与验证](music.md) |
| NewAPI 监控 | [配置、统计口径与余额重置](newapi-monitor.md) |

## 代码与板级资料

| 位置 | 内容 |
|---|---|
| [`internal/core`](../internal/core/) | 应用接口、事件和生命周期的实际契约 |
| [`internal/bootstrap/apps.go`](../internal/bootstrap/apps.go) | 应用工厂、共享服务、页面与终端命令装配 |
| [`internal/platform`](../internal/platform/) | 可复用的硬件、网络及音频能力 |
| [`board/lyra-zero-w`](../../board/lyra-zero-w/README.md) | 设备树、固件、内核模块及板级适配 |
| [`hardware`](../../hardware/Pinout.md) / [`sandbox`](../../sandbox/README.md) | 原理图、独立试验与历史验证；旧参数不自动等于当前默认值 |
| [`patches`](../patches/README.md) | 固定版本 LVGL 的本地补丁及维护说明 |

## 验证记录

记录对应当次产物与环境，不能视为当前板端状态或新版本的验收结果。原始证据保存在被 Git 忽略的 `build/`，仓库只保留脱敏总结。

- [NewAPI 看板与余额重置验证（2026-11-15）](reports/newapi-dashboard-2026-11-15.md)
- [米家属性读取修复（2026-11-15）](reports/mijia-properties-2026-11-15.md)

`secret.md` 是本地配置导入来源，不属于公开文档。凭据、对话、账号信息与原始截图不进入文档或仓库。
