# CosyVoice3 语音合成接口与客户端示例

本文记录 2026-10-17 核对的 `Fun-CosyVoice3-0.5B-2512` 部署接口，根据服务仓库的 `deployment.json`、FastAPI 路由及 `verify.py` 整理，不代表端点当前可用性。文中的部署文件、容器及 `AGENTS.md` 均属于外部服务环境；本项目客户端见 [`internal/speech/tts.go`](../internal/speech/tts.go)，应用接入与诊断见 [PicoClaw](picoclaw.md)。

## 1. 服务入口与鉴权

| 项目 | 当前配置 |
| --- | --- |
| HTTPS 地址 | `https://cosyvoice.zi-cloud.net`；主节点本机运维可用 `http://127.0.0.1:20005` |
| 模型 | `Fun-CosyVoice3-0.5B-2512`，FP32 |
| 鉴权 | `Authorization: Bearer <key>` 或 `X-API-Key: <key>` |
| 主节点密钥文件 | `/home/spark/.config/cosyvoice/api-key`（对应 `deployment.json` 的 `api.key_file`） |
| 请求格式 | POST `multipart/form-data`，参考音频作为文件上传 |
| 响应格式 | `application/octet-stream`，24 kHz、单声道、signed PCM16 little-endian，无 WAV 头 |

密钥由客户端从受保护文件读取，不写入源码、URL、命令行参数值或日志。远程客户端使用其本地的密钥文件路径。两种认证方式任选一种即可；当前中间件在任一种凭证正确时放行，客户端无需同时发送。

公开接口：

| 方法与路径 | 用途 |
| --- | --- |
| `GET /health` | 就绪检查，返回 `{"status":"ok","model":"Fun-CosyVoice3-0.5B-2512","sample_rate":24000,"precision":"fp32"}` |
| `GET /openapi.json` | 当前路由与表单字段定义 |
| `GET /docs`、`GET /redoc` | 交互式接口文档 |

所有 `/inference_*` 路径均在解析表单前鉴权。这里使用 CosyVoice 原生路由，不提供 OpenAI `/v1/audio/speech`。

## 2. 合成接口

以下三个 POST 接口已完成既有功能验收。所有列出的文本和文件字段均必填，`stream` 可省略，默认 `true`；发送表单字符串 `true` / `false` 可显式控制模型流式生成。请求中不需要 `model`、`voice` 或 `response_format`。

| 接口 | 表单字段 | 用途 |
| --- | --- | --- |
| `/inference_zero_shot` | `tts_text`、`prompt_text`、`prompt_wav`、可选 `stream` | 根据参考音频及准确转写克隆音色 |
| `/inference_cross_lingual` | `tts_text`、`prompt_wav`、可选 `stream` | 跨语言合成，无需参考音频转写 |
| `/inference_instruct2` | `tts_text`、`instruct_text`、`prompt_wav`、可选 `stream` | 根据参考音色和自然语言指令合成 |

字段与 CosyVoice3 提示词约定：

- `tts_text`：待合成文本。zero-shot / instruct2 直接传正文；cross-lingual 按既有验收用法，将 `You are a helpful assistant.<|endofprompt|>` 放在正文前。
- `prompt_wav`：参考录音文件，建议使用不超过 30 秒、清晰单人声的 WAV。服务端解码、转单声道并重采样到内部所需采样率；参考文件不要求与输出的 24 kHz 相同。不要把文件路径字符串当作文件内容上传。
- `prompt_text`：`You are a helpful assistant.<|endofprompt|>` 加上参考录音的准确转写，不能填写待合成文本。
- `instruct_text`：指令放在结束标记前，如 `You are a helpful assistant. 请用温柔的语气说话。<|endofprompt|>`。

三种请求的文本字段示例（均另附 `prompt_wav` 文件）：

```text
zero-shot:
  tts_text = 你好，欢迎使用本地语音合成服务。
  prompt_text = You are a helpful assistant.<|endofprompt|>希望你以后能够做的比我还好呦。

cross-lingual:
  tts_text = You are a helpful assistant.<|endofprompt|>Welcome to the local speech service.

instruct2:
  tts_text = 你好，祝你有愉快的一天。
  instruct_text = You are a helpful assistant. 请用温柔的语气说话。<|endofprompt|>
```

路由还注册了 `/inference_sft`（`tts_text`、`spk_id`）和 `/inference_instruct`（另需 `instruct_text`），两者也有 `stream`。这些旧接口依赖对应模型及预置说话人，不能作为本 CosyVoice3 部署的可用能力。所有合成路由也注册了 GET，但参数仍从表单读取，客户端统一用 POST。

## 3. 音频接收与错误处理

响应体是连续的 PCM 字节，每秒 `24000 × 1 × 2 = 48000` 字节。不能直接将响应保存为 `.wav` 后缀并当作 WAV；需要封装文件头，或交给明确指定格式的 PCM 播放器。

`stream=true` 时边生成边返回音频；`stream=false` 仍返回相同 PCM 格式，并非 JSON 或 WAV。服务端通过 `StreamingResponse` 传输两种模式。HTTP 读取块边界不等于模型音频块边界，也不保证对齐 2 字节采样；实时播放时应保留奇数字节尾部，与下一块拼接。短文本可能只有一个生成块。

服务端返回 `X-Accel-Buffering: no`；如接入反向代理，应关闭响应缓冲并设置足够的读取超时。首音频延迟、总耗时、音频时长和 RTF（总耗时 / 音频时长）是合适的观测指标。

| 情况 | 当前行为 / 客户端处理 |
| --- | --- |
| 缺失或错误密钥 | HTTP 401，`{"detail":"Unauthorized"}`，带 `WWW-Authenticate: Bearer` |
| 有效密钥但缺少必填表单字段，或字段无法解析 | HTTP 422，FastAPI `detail` 校验信息 |
| 参考录音无法解码、模型推理异常 | 可能返回 5xx，或在响应头已发出后中断音频流；没有统一 JSON 错误协议 |
| 网络异常或读取超时 | 将结果视为不完整；不能把已有音频字节当作完整合成成功 |

客户端先检查 HTTP 状态，再消费音频；结束后检查非空且总字节数为偶数。自动重试会重新合成，可能重复播放，宜由上层明确决定。

## 4. Python 客户端：三个接口、流式接收、保存 WAV

客户端只需 Python 3 和 `requests`，无需安装 Torch 或模型。可在客户端虚拟环境安装 `python3 -m pip install requests`。将下面代码保存为 `tts_client.py`；示例会逐块接收 PCM 并写入 WAV，文件在请求完成、关闭后可正常播放。它不包含扬声器实时播放逻辑。

```python
import argparse
from pathlib import Path
import wave

import requests


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--base-url", default="https://cosyvoice.zi-cloud.net")
    p.add_argument("--api-key-file", type=Path,
                   default=Path("/home/spark/.config/cosyvoice/api-key"))
    p.add_argument("--mode", choices=["zero-shot", "cross-lingual", "instruct2"],
                   default="zero-shot")
    p.add_argument("--prompt-wav", type=Path, required=True)
    p.add_argument("--text", required=True)
    p.add_argument("--reference-text", help="参考录音的准确转写，zero-shot 必填")
    p.add_argument("--instruction", default="请用温柔的语气说话。")
    p.add_argument("--non-stream", action="store_true")
    p.add_argument("--output", type=Path, default=Path("output.wav"))
    a = p.parse_args()
    if a.mode == "zero-shot" and not a.reference_text:
        p.error("zero-shot 需要 --reference-text")

    prefix = "You are a helpful assistant."
    marker = "<|endofprompt|>"
    fields = {"tts_text": a.text,
              "stream": "false" if a.non_stream else "true"}
    if a.mode == "zero-shot":
        fields["prompt_text"] = prefix + marker + a.reference_text
    elif a.mode == "cross-lingual":
        fields["tts_text"] = prefix + marker + a.text
    else:
        fields["instruct_text"] = prefix + " " + a.instruction + marker
    route = "/inference_" + a.mode.replace("-", "_")
    key = a.api_key_file.read_text().strip()
    headers = {"Authorization": "Bearer " + key}
    received = 0
    created = False
    try:
        with a.prompt_wav.open("rb") as reference:
            with requests.post(
                a.base_url.rstrip("/") + route, headers=headers, data=fields,
                files={"prompt_wav": ("reference.wav", reference, "audio/wav")},
                stream=True, timeout=(5, 240),
            ) as response:
                response.raise_for_status()
                if response.headers.get("Content-Type", "").split(";")[0] != "application/octet-stream":
                    raise RuntimeError("Unexpected audio response type")
                with a.output.open("xb") as output:
                    created = True
                    with wave.open(output, "wb") as wav:
                        wav.setnchannels(1)
                        wav.setsampwidth(2)
                        wav.setframerate(24000)
                        pending = b""
                        for chunk in response.iter_content(chunk_size=None):
                            pending += chunk
                            aligned = len(pending) // 2 * 2
                            if aligned:
                                wav.writeframesraw(pending[:aligned])
                                received += aligned
                                pending = pending[aligned:]
                        if pending or received == 0:
                            raise RuntimeError("Incomplete or empty PCM response")
    except BaseException:
        # 失败时删除本次创建的文件，避免后续误用残缺音频。
        if created:
            a.output.unlink(missing_ok=True)
        raise
    print(f"Saved {a.output}, {received / 48000:.2f} seconds")


if __name__ == "__main__":
    main()
```

示例拒绝覆盖已有输出文件，调用时选择新的 `--output` 路径。`--non-stream` 控制服务端生成方式，代码中的 `requests` 参数 `stream=True` 只控制客户端逐块读取，两者含义不同。

```bash
python3 tts_client.py --mode zero-shot \
  --prompt-wav reference.wav --reference-text '希望你以后能够做的比我还好呦。' \
  --text '你好，欢迎使用本地语音合成服务。' --output zero-shot.wav

python3 tts_client.py --mode cross-lingual \
  --prompt-wav reference.wav \
  --text 'Welcome to the local speech service.' --output english.wav

python3 tts_client.py --mode instruct2 --non-stream \
  --prompt-wav reference.wav --instruction '请用温柔的语气说话。' \
  --text '你好，祝你有愉快的一天。' --output gentle.wav
```

只有使用官方示例参考录音时，上述参考转写才适用。主节点可按 `AGENTS.md` 从容器提取 `asset/zero_shot_prompt.wav`；自己的录音必须替换转写。

## 5. curl 示例与既有验收程序

通过公网 HTTPS 调用；以下示例使用主节点密钥文件路径，远程客户端需改为本地密钥文件路径。密钥经标准输入交给 curl，不展开到进程参数。响应存为裸 PCM：

```bash
python3 - <<'PY' | curl --silent --show-error --fail-with-body \
  --header @- \
  --form-string 'tts_text=你好，欢迎使用本地语音合成服务。' \
  --form-string 'prompt_text=You are a helpful assistant.<|endofprompt|>希望你以后能够做的比我还好呦。' \
  --form 'prompt_wav=@reference.wav;type=audio/wav' \
  --form-string 'stream=true' \
  --output output.pcm https://cosyvoice.zi-cloud.net/inference_zero_shot
from pathlib import Path
print("Authorization: Bearer " + Path("/home/spark/.config/cosyvoice/api-key").read_text().strip())
PY

# curl 成功退出后再播放；PCM 不能自动探测采样率。
ffplay -nodisp -autoexit -f s16le -ar 24000 -ac 1 output.pcm
```

curl 失败时 `output.pcm` 可能是错误体或残缺音频，不应播放。健康检查无需密钥：`curl --fail https://cosyvoice.zi-cloud.net/health`。

需要重跑已有功能验收时，可使用 `verify_auth.py` 与 `verify.py`，参数及范围见 `AGENTS.md`。既有验收覆盖三种主要模式、两种鉴权、流式与非流式；未提供试听/ASR 内容评分、并发容量或长文本性能保证。
