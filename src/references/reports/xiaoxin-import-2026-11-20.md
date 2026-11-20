# 小新桌宠生成与实机导入（2026-11-20）

## 素材

使用 `pets/xiaoxin/` 中用户提供的坐姿、仰躺和睡姿照片。火山方舟 `doubao-seedream-5-0-flash-260915` 生成三张黑底参考图，`doubao-seedance-2-0-fast-260128` 逐状态生成视频，未使用 welcome 复制填充其他状态。

交付 11 类：welcome、idle.default、idle.playing、idle.wandering、idle.jumping、working、waiting_user、done、error、touch.lick、touch.what。每段约 5 秒，板端均为 640×480、15 fps、H.264 Baseline / yuv420p、静音 MP4。清单加视频共 1,433,454 字节，约 1.37 MiB。

首次跳跃结果在最高点裁到耳朵，单独用缩小并增加上方留白的参考图重新生成，缩小跳跃幅度。最终跳跃任务为 `cgt-20261120000858-h9i18`；76 帧顶部 24 行最大亮度 Y=18，另查看 5 fps 抽帧和腾空帧。共调用三次参考图生成、十二次视频生成，最终保留十一段。用户于 2026-11-20 确认实测正常后，按要求清理被替换版本及过程文件，保留最终原片、板端包、实际输入参考图和制作记录。未做币种费用估算，最终任务的 API 返回用量保存在生成记录中。

[素材说明与预览](../../../pets/xiaoxin/README.md)、[板端素材包](../../../pets/xiaoxin/device-pack/manifest.json)。当次本地离线 ZIP 的 SHA-256 如下；ZIP 不纳入 Git，可按素材说明重新打包：

```text
74f70b20298bb9a04ce65efc6f41274994da46af1996a03260b47d24dd982192
```

本地生成记录只保存模型、任务 ID、提示词、用量及哈希；不保存 API Key，已移除临时签名下载地址。

## 导入与验证

Lyra Zero W / RK3506B，板端 FFmpeg 4.4.4。未修改或重新构建播放器，沿用已安装二进制：

```text
cb117416e266fb3c57de02b2c992dfc5e397285adb36a87423c9efcf08754382
```

先核对 SMB 候选 JSON、secrets、agent、assets 与当前代次相同，确认没有未应用草稿。通过 SSH 写入 SMB 的服务端候选目录 `config-share/assets/pets/xiaoxin/`，只将 `ui.pet.directory` 改为 `assets/pets/xiaoxin`，随后通过实际设置页面“应用配置并重启”完成校验及原子切换。跳跃修正也通过相同配置应用流程生效。最终代次 `cfg-1974155926`。

- 最终包 11 段 MP4 与本地哈希逐一相符，全部由板端 FFmpeg 完整解码通过，ffprobe 确认单视频流和格式。
- 其他配置、secrets、agent、既有 assets（含西高地）、私有 LAN 配对记录和界面偏好保持一致；用哈希核对，不发布私有内容。
- 真实板端以 `-page screensaver` 启动，帧缓冲可见“小新”，解码命令指向最终代次的 `idle.jumping.mp4`。
- 使用经身份校验的 evdev 触摸注入退出屏保，返回组件中心，FFmpeg 进程归零。独立预览结束后恢复正式常驻服务，运行状态正常。

原始截图、检查结果和文件哈希保存在被 Git 忽略的 `src/build/reports/xiaoxin-2026-11-20/`。本次验证 SSH 写入候选资源与实际配置应用链路，没有重新测试 Windows SMB 网络上传、物理 SW3 或官方 Desktop 的真实 Agent 状态推送。板端显示仍等待 Desktop 配对。两段触摸素材已导入，当前触摸仍用于退出屏保；视频未保证严格首尾无缝。
