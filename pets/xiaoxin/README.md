# 小新桌宠素材

使用本目录三张原始照片生成，原照片保留不变。先分别提取坐姿、仰躺、睡姿的黑底参考图，再为每种状态单独生成视频。

- 服务：火山方舟，北京区域。
- 参考图模型：`doubao-seedream-5-0-flash-260915`。
- 视频模型：`doubao-seedance-2-0-fast-260128`。
- 每段约 5 秒，4:3、无音频；板端归一化为 H.264 Baseline / yuv420p、640×480、15 fps。
- 生成时间：2026-11-19 至 2026-11-20（北京时间）。

| 文件名 | 状态 |
|---|---|
| `welcome` | 欢迎 |
| `idle.default` | 睡眠待机 |
| `idle.playing` | 玩耍 |
| `idle.wandering` | 闲逛 |
| `idle.jumping` | 小跳 |
| `working` | 工作中 |
| `waiting_user` | 等待确认 |
| `done` | 完成 |
| `error` | 错误反馈 |
| `touch.lick` | 亲昵反应 |
| `touch.what` | 好奇反应 |

[浏览最终云端视频](generated/index.html)。用户于 2026-11-20 确认板端实测正常，中间过程文件已清理。当前保留：

- 三张 `xiaoxin_*.jpg`：原始照片。
- `generated/videos/`：11 段最终云端原片，作为后续转码母版。
- `generated/references/*-768.png`：实际用于生成视频的四张参考图，包括增加留白的跳跃首帧。
- `generated/recipe.json`：生图提示词、输入照片顺序、模型和请求参数。
- `generated/generation.json`：最终各状态的视频提示词、参考姿势、任务 ID、用量和原片哈希。
- `generated/validation.json`：最终板端视频格式检查。
- `generated/index.html`：本地视频预览。

后续制作其他形象可参考[制作流程](../README.md)。重复转码目录、旧跳跃版本、调试预览及未使用的高分辨率参考图已删除。

[`device-pack/`](device-pack/) 是离线导入包，仅含清单和 11 段板端视频，不包含照片、API Key 或云端下载地址。将包内容放进 SMB 的 `assets/pets/xiaoxin/`，设置 `ui.pet.directory` 为 `assets/pets/xiaoxin`，再在设备设置中应用配置并重启。

`xiaoxin-device-pack.zip` 为本地打包产物，不纳入 Git。需要 ZIP 时，从项目根目录执行：

```sh
cd pets/xiaoxin/device-pack
python3 -m zipfile -c ../xiaoxin-device-pack.zip manifest.json videos
```

当前播放器在进入屏保时播放欢迎，空闲时轮换 `idle.*`，根据 Desktop 的状态切换工作、等待、完成和错误。两段触摸素材用于空闲互动，短按 SW3 返回原页面；关闭显示待机时，首次触摸只唤醒。生成视频并非严格首尾无缝循环。
