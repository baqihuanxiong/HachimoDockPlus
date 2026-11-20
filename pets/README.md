# 桌宠素材

每个形象使用独立目录，保留原始照片、最终云端视频、板端导入包和制作记录。现有示例：[小新](xiaoxin/README.md)，用户于 2026-11-20 确认实测正常。

## 制作其他形象

以下为小新这次实际使用的流程和参数。模型名称记录当次成功调用的版本；重新制作时按账户可用模型配置。

1. 准备同一只宠物的全身照片，覆盖坐姿、仰躺和睡姿；照片清楚展示耳朵、爪子、尾巴及毛色。
2. 分别制作三种姿势的黑底参考图。把目标姿势照片放在第一张，其余照片用于保持身份一致。生图提示词和参数见 [recipe.json](xiaoxin/generated/recipe.json)；制作新形象时替换名字、外形描述和原照片路径。
3. 将参考图等比缩放并填充为 768×576，检查全身完整、背景与留白。跳跃应额外留出上方空间；小新的修正版使用 `scale=538:404,pad=768:576:115:140:black` 处理普通坐姿参考图，作为跳跃首帧。
4. 为每个状态分别提交视频任务。参考 [generation.json](xiaoxin/generated/generation.json) 中各状态的 `reference` 和完整 `prompt`。替换新宠物的名字、物种、毛色及特征，保留固定镜头、黑底、无道具、身体完整等约束。当前例子共 11 类状态；待机使用多个动作，不用 welcome 复制填充。
5. 保存任务 ID 后轮询并下载视频，先抽查欢迎，再批量生成其余状态。小新使用最多两个并发任务。提交结果不明时先核查已有任务，避免直接重复提交产生费用。API Key 只在运行时提供，不写进素材目录；下载完成后移除临时签名地址。
6. 检查整个运动过程，尤其是跳跃最高点和尾巴活动范围；检查身份、肢体及画面边缘。异常只重做对应状态。每段约 5 秒，生成结果未保证严格首尾无缝。
7. 将最终云端原片保存为 `generated/videos/<状态>.mp4`，通过下方工具生成板端包，再核对解码与实机播放。导入后可清理重复转码目录、调试预览、被替换版本及未使用的高分辨率参考图。

## 请求约定

本次使用火山方舟北京区域 `https://ark.cn-beijing.volces.com`，请求头使用运行时 API Key 的 Bearer 认证。

- 参考图：`POST /api/v3/images/generations`，参数见 recipe 的 `image_request`；`image` 按对应 `input_photos` 顺序传图片 data URL，`prompt` 使用对应姿势的完整提示词。
- 视频：`POST /api/v3/contents/generations/tasks`，参数见 recipe 的 `video_request`；`content` 包含 `type: text` 的状态提示词，以及 `type: image_url`、`role: first_frame` 的参考图 data URL。
- 查询：`GET /api/v3/contents/generations/tasks/<任务 ID>`，成功后下载 `content.video_url`。提示词、任务 ID、返回用量和原片哈希写入 generation 记录，API Key 不写入记录。

## 板端包

从项目根目录执行，输出目录必须尚不存在：

```sh
python3 src/tools/import-pet.py pets/<形象>/generated \
  --name '<显示名称>' --output pets/<形象>/device-pack
```

工具归一化为 640×480、15 fps、H.264 Baseline / yuv420p、黑底静音 MP4。主机无 FFmpeg 时使用项目开发镜像。不要对云端原片使用 `--copy`，该选项只适合已经完成归一化的视频。

通过 SMB 将包内容放到 `assets/pets/<形象>/`，修改共享配置中的 `ui.pet.directory`，在设备设置中“应用配置并重启”。保留原有 `ui.pet` 其他字段。完整导入说明见[桌宠屏保](../src/references/pet-screensaver.md)。
