# 火山引擎豆包语音

PicoClaw 支持自定义语音接口和火山引擎豆包语音，ASR / TTS 可以独立选择。未填写 `provider` 时继续使用原来的自定义接口，无需迁移旧配置。下面字段放在 `apps.picoclaw` 下；旧版顶层 `asr` / `tts` 配置也兼容，但不能与 `apps.picoclaw` 同时使用。

```json
{
  "asr": {
    "provider": "volcengine",
    "key_file": "secrets/doubao.key",
    "resource_id": "volc.seedasr.sauc.duration"
  },
  "tts": {
    "provider": "volcengine",
    "key_file": "secrets/doubao.key",
    "resource_id": "seed-tts-2.0",
    "speaker": "zh_female_vv_uranus_bigtts",
    "speech_rate": 0
  }
}
```

`url`、`resource_id` 和 TTS `speaker` 省略时使用上面对应的默认值。`provider` 只接受 `custom` / `volcengine`。密钥来自新版豆包语音控制台 API Key，使用 `X-Api-Key` 请求头，ASR 和 TTS 可以共用有对应服务权限的 Key，也可分别指定密钥文件。密钥文件应为 `600` 权限，目录为 `700`；不要把密钥写进 JSON、URL、源码或日志。

默认设置与本仓库官方 HachimoDock 参考版本保持一致：流式识别 2.0、语音合成 2.0 和 Vivi 2.0 音色。音色可替换为相应资源支持的 ID，`speech_rate` 范围为 -50～100，0 为正常语速。豆包 TTS 不需要 `prompt_wav`、`prompt_text`、`instruction` 或 `mode`；这些字段仅用于自定义 CosyVoice 接口。

## 音频与协议

- ASR：`wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async`，16 kHz、单声道 PCM16 LE；200 ms 分片，gzip 二进制帧，松开录音后发送最终帧，只有收到服务端最终标志才提交识别文字。预览使用累计快照，断线不会把临时文本提交给智能体。识别语言由服务自动处理，旧的 `language` 提示只用于自定义接口。
- TTS：`https://openspeech.bytedance.com/api/v3/tts/unidirectional`，一次提交文本，增量解码 JSON / Base64 音频，输出 24 kHz、单声道 PCM16 LE，复用现有 ALSA 播放与停止播报流程。该 HTTP 接口也是官方 HachimoDock 使用的豆包传输方式之一，本实现统一使用它，不复用官方桌面版的双向 WebSocket 会话。
- 云端错误只显示 HTTP 状态或服务错误码，不回显响应正文；取消会关闭录音流、连接或响应体。未收到完整 TTS 结束标志、空音频、奇数字节或损坏响应均报错。

配置通过 SMB 编辑时，同时把新密钥放入共享 `secrets/`，然后在「设置 → 系统」应用配置并重启；仅部署新二进制不会覆盖既有服务配置。详见 [配置共享](configuration-sharing.md)。

## 验证

开发主机执行：

```sh
tools/dev.sh sh -c 'go test -race -p 2 -tags stdjson,goolm ./... && go vet -tags stdjson,goolm ./...'
tools/dev.sh tools/build.sh arm
```

板端命令（输出文件必须尚不存在）：

```sh
/opt/hachimodock/current/dock -config /opt/hachimodock/config.json \
  -tts '你好，豆包语音测试。' -pcm-output /tmp/doubao-24k.pcm
# -asr-pcm 输入必须是 16 kHz 单声道 PCM16 LE，不能直接使用上面的 24 kHz 文件。
/opt/hachimodock/current/dock -config /opt/hachimodock/config.json -asr-pcm /tmp/test-16k.pcm
```

协议来源：[官方 HachimoDock ASR](../../HachimoDock/pc/src-tauri/src/volcengine_asr.rs)、[官方 HachimoDock HTTP TTS](../../HachimoDock/pc/src-tauri/src/doubao_tts_http.rs)、[火山引擎流式 ASR](https://docs.volcengine.com/docs/doubao-voice/large-model-streaming-automatic-speech-recognition-api-1?lang=zh)、[火山引擎 HTTP 流式 TTS](https://docs.volcengine.com/docs/doubao-voice/unidirectional-streaming-text-to-speech-http?lang=zh)。
