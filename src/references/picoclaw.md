# PicoClaw 对话

组件 ID 为 `picoclaw`。通过 Go 包嵌入 [PicoClaw v0.3.1](https://github.com/sipeed/picoclaw/tree/v0.3.1) 的 AgentLoop，无需单独运行 PicoClaw 服务。模型、ASR 和 TTS 使用配置的远程服务；工具与技能在板端运行，见 [智能体扩展](picoclaw-extensions.md)。

## 操作与回复

| 输入 | 行为 |
|---|---|
| 输入框 / 发送按钮 / SW2 | 编辑并发送文字 |
| 按住 SW1、松开 | 开始录音、结束识别；最终文本留在输入框，确认后再发送 |
| 回复中按 SW1 | 取消当前回复与播报 |
| 语音播报开关 | 默认开启；关闭立即停止合成或播放，文字接收继续 |
| 停止播报 | 只停止当前语音，保留文字及下一轮的播报设置 |
| 新建对话 | 清空当前页面消息，使用独立会话键 |
| SW3 | 先关闭键盘或弹层，再返回组件中心并取消前台任务 |

录音最长 30 秒，每轮总时限 3 分钟。语音链路为 PCM16 / 16 kHz 单声道采集 → FunASR 定稿 → PicoClaw → CosyVoice → PCM16 / 24 kHz 单声道播放。回复文字以流式增量显示，支持标题、列表、加粗与代码；最终答复完成后才合成语音，目前没有全双工对话。

“处理过程”保留工具调用前的公开说明，执行中默认展开，答复完成后自动折叠，取消或失败也保留已收到的内容。可手动展开历史过程；私有推理不进入该区域，只有最终答复参与播报。

离开再进入保留会话；播报开关在当前应用实例内保留，新建对话不重置，进程重启后恢复默认。上翻阅读时保持位置，位于底部时跟随新内容。当前屏幕键盘只支持英文与符号，中文可通过语音输入。

## 配置与数据

参考 [`config.example.json`](../config.example.json) 的 `apps.picoclaw`，填写 `llm`、`asr` 和 `tts` 的地址与密钥文件。打开应用时会验证这三组配置并读取密钥；缺少声卡时仍可文字对话，语音依赖不可用时显示降级提示。

- LLM：配置兼容服务地址及模型名；工作区、会话与记忆默认位于 `data_dir/apps/picoclaw/`。
- ASR：WebSocket 协议和最终文本确认规则见 [FunASR 接口](asr_interface.md)。
- TTS：默认 `instruct2`，需要 `prompt_wav` 和语气指令；`zero_shot` 还需准确的参考转写 `prompt_text`。详见 [CosyVoice 接口](tts_interface.md)。
- 扩展：`extensions_dir` 默认是配置目录下的 `agent/`；提示词、技能、CLI、MCP 和私有运行数据布局见 [扩展说明](picoclaw-extensions.md)。

旧版顶层 `llm/asr/tts` 配置在内存中转换，继续使用原数据目录，不自动改写文件或搬动会话。手工迁移到 `apps.picoclaw` 时，用应用的 `data_dir` 指向原目录，或显式迁移数据，见 [配置与持久化约定](app-development.md#4-配置与持久化)。

音频格式、功放及全局音量复用 [硬件基线](hardware-adaptation.md#音频功放和全局音量)，输出选择见 [系统音频配置](configuration-sharing.md#资源文件与音频输出)。

## 服务诊断

在 `src/` 下执行，需先准备配置与 native 构建：

```sh
tools/dev.sh build/native/dock -config .local/config.json -ask '用一句话介绍自己'
tools/dev.sh build/native/dock -config .local/config.json -tts '你好' -pcm-output build/hello-24k.pcm
# 无 WAV 头的 PCM16 LE、单声道、16 kHz 输入
tools/dev.sh build/native/dock -config .local/config.json -asr-pcm build/input-16k.pcm
```

`-tts` 只生成文件，不播放音频；输出文件必须尚不存在。服务诊断不能代替页面、麦克风与扬声器的整链路验证，验收按 [实机测试流程](board-validation.md) 执行。
