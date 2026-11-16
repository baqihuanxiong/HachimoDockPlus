# Fun-ASR-Nano 实时语音识别接口与客户端示例

本页描述自定义服务；新增火山引擎豆包语音的配置与协议见 [豆包语音](doubao-voice.md)。

本文记录 2026-10-17 核对的 `Fun-ASR-Nano-2512` 部署接口，根据服务仓库的 `deployment.json`、`serve_ws.py`、`funasr.bin.realtime_ws` 与 `verify.py` 整理，不代表端点当前可用性。文中的部署文件、容器及 `AGENTS.md` 均属于外部服务环境；本项目客户端见 [`internal/speech/asr.go`](../internal/speech/asr.go)，应用接入与诊断见 [PicoClaw](picoclaw.md)。

## 1. 服务入口与鉴权

| 项目 | 当前配置 |
| --- | --- |
| WebSocket 地址 | `wss://funasr-nano.zi-cloud.net`；主节点本机运维可用 `ws://127.0.0.1:20006` |
| 健康检查 | `GET https://funasr-nano.zi-cloud.net/health`，公开，模型加载完成后返回 HTTP 200、文本 `ok` |
| 模型 | `Fun-ASR-Nano-2512`，服务端固定选择，客户端不传模型名 |
| 握手鉴权 | 单个 `Authorization: Bearer <key>` 请求头 |
| 主节点密钥文件 | `/home/spark/.config/funasr/api-key`（对应 `deployment.json` 的 `api.key_file`） |
| 音频输入 | 二进制消息，16 kHz、单声道、signed PCM16 little-endian，无 WAV 头 |
| 文本输出 | UTF-8 JSON 文本消息，含控制事件、临时文本和确认句子 |

密钥由客户端从受保护文件读取，不写入源码、URL、命令行参数值或日志。远程客户端使用其本地密钥文件路径。缺失、错误或重复的 `Authorization` 在升级为 WebSocket 前返回 HTTP 401、文本 `Unauthorized`。本服务不支持 `X-API-Key` 或查询参数鉴权。

当前部署提供实时 WebSocket 协议，没有文件上传 HTTP 转写接口，也没有 OpenAI `/v1/audio/transcriptions`。普通浏览器 WebSocket API 无法自定义 `Authorization` 请求头；浏览器接入需要由受控业务后端代理建立带鉴权的上游连接。

## 2. 会话流程与音频格式

1. 建立带 Bearer 请求头的 WebSocket 连接。
2. 发送文本消息 `START`，等待 `{"event":"started"}`。
3. 可发送 `LANGUAGE:中文` 或 `LANGUAGE:English`，等待 `language_set`；当前部署默认中文。
4. 按采集节奏发送二进制 PCM，同时持续接收识别 JSON。推荐每 100 ms 一帧：1600 个样本，即 3200 字节。
5. 录音结束后发送文本 `STOP`，继续读取最终识别结果 `is_final:true` 和随后 `{"event":"stopped"}`。
6. 收到 `stopped` 后关闭连接，或重新 `START` 开始下一次录音。

命令是原始文本帧，例如 `START`，不是 JSON 字符串 `"START"`，也不是 `{"command":"START"}`。音频必须是二进制帧；不发送 Base64、WAV 文件头、MP3 或 WebM/Opus 数据。WAV 应先读取样本，其他格式应先在客户端解码和重采样。每帧字节数必须为偶数。

当前由服务端 VAD 判断停顿并确认句子。静音期间也应持续发送采集到的 PCM，跳过静音会改变音频时间轴及分句行为。100 ms 是输入帧建议，不代表每 100 ms 都返回识别结果；当前 decode interval 为 0.48 秒，还受有效音频长度、VAD、推理和排队影响。

`START` 会清空当前录音的音频与识别结果；不要在旧录音未结束时用它做心跳。语言和热词属于连接内的会话配置，`START` 不重置这些配置，复用连接时建议每次显式设置。不同连接的配置隔离。

## 3. 控制消息

| 客户端文本消息 | 服务端事件 | 说明 |
| --- | --- | --- |
| `START` | `{"event":"started"}` | 重置录音结果并开始接收音频 |
| `LANGUAGE:中文` | `{"event":"language_set","language":"中文"}` | 设置识别语言提示，亦可用 `English`；空值清除提示 |
| `STOP` | 最终识别 JSON，然后 `{"event":"stopped"}` | 结束当前录音，保留连接；无音频或未激活时可能只有 `stopped` |
| `HOTWORDS:港机,集装箱` | `{"event":"hotwords_set","hotwords":["港机","集装箱"]}` | 设置逗号分隔的解码热词，空列表清除 |
| `POSTPROCESS_HOTWORDS:港鸡=>港机` | `{"event":"postprocess_hotwords_set","postprocess_hotwords":{"港鸡":"港机"}}` | 对定稿文本做显式替换，多条可用换行或逗号分隔 |
| `COMMIT` | `{"event":"error","error":"COMMIT requires --endpoint-mode client"}` | 当前 `endpoint_mode=server` 不支持该命令 |

语言设置和 START/STOP 已有功能验收。热词命令来自当前固定实现，尚无本部署专项热词效果验收；解码热词与定稿文本替换是不同功能。客户端应等待对应确认事件后再送音频。未知命令当前可能被忽略，没有统一的参数校验错误响应。

## 4. 识别结果与显示规则

临时结果示意（文本和时间仅作结构示例）：

```json
{
  "sentences": [{"text": "欢迎使用语音识别。", "start": 120, "end": 2100}],
  "partial": "现在开始",
  "partial_start_ms": 2600,
  "duration_ms": 4200,
  "is_final": false
}
```

STOP 定稿示意：

```json
{
  "sentences": [
    {"text": "欢迎使用语音识别。", "start": 120, "end": 2100},
    {"text": "现在开始测试。", "start": 2600, "end": 4800}
  ],
  "partial": "",
  "partial_start_ms": 0,
  "duration_ms": 5000,
  "is_final": true
}
```

| 字段 | 含义 |
| --- | --- |
| `sentences` | 本次 START 以来已确认句子的累计快照，每句含 `text`、`start`、`end` |
| `partial` | 当前可修订的临时文本，可能为空或在下一次结果中变化 |
| `partial_start_ms` | 当前临时文本对应的音频起始时间；定稿时为 0 |
| `duration_ms` | 服务端已接收的 PCM 总时长，按样本数计算，不是请求耗时 |
| `is_final` | 当前录音是否已定稿；正常有音频的 STOP 返回 true |

时间均为相对本次 START 的毫秒数，句子时间是分段边界，不是逐词精确对齐。当前关闭说话人识别，不应依赖 `spk` 字段。

收到每个识别 JSON 后，用新的 `sentences` 和 `partial` 替换当前显示状态，不要逐次追加累计句子或 partial，否则会重复。可将 `"".join(s["text"] for s in result["sentences"]) + result["partial"]` 用作简单预览。最终文本只取最后的确认句子；若业务需要英文句子间空格，应在展示层按语言处理。

静音可以得到空 `sentences` 和空 `partial`，这属于正常识别结果。断线、超时或只有 `stopped` 而没有预期定稿时，应明确标记未完成，不能把最后一个 partial 当作最终答案。服务端异常可能直接关闭连接，没有统一 JSON 错误格式；重新连接不会自动续传旧会话。

## 5. Python 客户端：按录音速度发送 WAV，同时接收文字

客户端只需 Python 3 与 `websockets`，无需安装 FunASR、Torch 或 GPU 依赖。下面使用服务验收版本 `websockets==17.1` 的 `additional_headers` 接口，可在客户端虚拟环境安装：

```bash
python3 -m pip install websockets==17.1

# 若输入不是 16 kHz、单声道、PCM16 WAV，先转换（需要 ffmpeg）。
ffmpeg -i input.mp3 -ar 16000 -ac 1 -c:a pcm_s16le input-16k.wav
```

将下面代码保存为 `asr_client.py`。示例用 WAV 模拟实时采集，每 100 ms 发送一帧，并行接收文本；不会一次性发送完整文件。`FIRST_TEXT` 会显示首个非空结果到达时，是否还有音频帧尚未发送。

```python
import argparse
import asyncio
import json
from pathlib import Path
import time
import wave

from websockets.asyncio.client import connect


async def run(a):
    key = a.api_key_file.read_text().strip()
    with wave.open(str(a.wav), "rb") as wav:
        if (wav.getnchannels(), wav.getsampwidth(), wav.getframerate(),
                wav.getcomptype()) != (1, 2, 16000, "NONE"):
            raise ValueError("Input must be mono, PCM16, 16000 Hz WAV")
        pcm = wav.readframes(wav.getnframes())
    if not pcm or len(pcm) % 2:
        raise ValueError("Empty or invalid PCM input")

    async with connect(
        a.url, additional_headers={"Authorization": "Bearer " + key},
        open_timeout=10, max_size=4 * 1024 * 1024, ping_timeout=None,
    ) as ws:
        async def expect(name):
            message = json.loads(await asyncio.wait_for(ws.recv(), 20))
            if message.get("event") != name:
                raise RuntimeError(f"Expected {name}, received {message}")

        await ws.send("START")
        await expect("started")
        await ws.send("LANGUAGE:" + a.language)
        await expect("language_set")
        started = time.monotonic()
        sent_bytes = 0
        first_text = False
        final = None

        async def receive():
            nonlocal first_text, final
            while True:
                result = json.loads(await ws.recv())
                if result.get("event") == "error":
                    raise RuntimeError(result.get("error"))
                if result.get("event") == "stopped":
                    if final is None:
                        raise RuntimeError("Stopped without final transcript")
                    return final
                if "sentences" not in result:
                    continue
                preview = "".join(s["text"] for s in result["sentences"])
                preview += result.get("partial", "")
                if preview.strip() and not first_text:
                    first_text = True
                    print("FIRST_TEXT", round(time.monotonic() - started, 3),
                          "seconds; unsent_audio =", sent_bytes < len(pcm),
                          flush=True)
                # 每行是最新快照；UI 应替换显示，不能累计追加。
                print("FINAL" if result.get("is_final") else "UPDATE",
                      preview, flush=True)
                if result.get("is_final"):
                    final = result

        async def send():
            nonlocal sent_bytes
            for offset in range(0, len(pcm), 3200):
                target = started + offset / 32000
                await asyncio.sleep(max(0, target - time.monotonic()))
                frame = pcm[offset:offset + 3200]
                await ws.send(frame)
                sent_bytes += len(frame)
            # 最后一帧也要经过其实际采集时长后，才结束录音。
            await asyncio.sleep(max(0, started + len(pcm) / 32000 - time.monotonic()))
            await ws.send("STOP")

        sender = asyncio.create_task(send())
        receiver = asyncio.create_task(receive())
        try:
            # 示例超时预算：音频时长加 60 秒，不代表服务延迟承诺。
            _, final = await asyncio.wait_for(
                asyncio.gather(sender, receiver), len(pcm) / 32000 + 60)
        finally:
            for task in (sender, receiver):
                if not task.done():
                    task.cancel()
            await asyncio.gather(sender, receiver, return_exceptions=True)
        expected_ms = len(pcm) * 1000 // 32000
        if final["duration_ms"] != expected_ms:
            raise RuntimeError("Server audio duration does not match input")
        print(json.dumps(final, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    p = argparse.ArgumentParser()
    p.add_argument("wav", type=Path)
    p.add_argument("--url", default="wss://funasr-nano.zi-cloud.net")
    p.add_argument("--api-key-file", type=Path,
                   default=Path("/home/spark/.config/funasr/api-key"))
    p.add_argument("--language", default="中文")
    asyncio.run(run(p.parse_args()))
```

调用示例：

```bash
python3 asr_client.py input-16k.wav --language 中文
python3 asr_client.py english-16k.wav --language English \
  --api-key-file /path/to/local/asr-api-key
```

实际麦克风接入时，将 WAV 读取和按时发送循环替换为采集队列：采集端输出 16 kHz、单声道、PCM16，每 100 ms 入队 3200 字节，发送协程逐帧发送，接收协程保持不变。真实采集已按时间产生样本，不再额外按帧 sleep。停止采集后先发送剩余帧，再发 STOP，等待定稿和 stopped。

## 6. 验证与能力边界

健康检查：

```bash
curl --fail https://funasr-nano.zi-cloud.net/health
```

已有完整功能验收程序在运行镜像中，无需向宿主机安装推理依赖：

```bash
sudo -n docker exec funasr-nano python3 /opt/maas/verify.py \
  --url ws://127.0.0.1:10095 --output /tmp/funasr-verification.json
```

此命令会执行实际识别；需在服务空闲时按需运行。它覆盖中文、英文、静音、停顿分句、连接复用、两会话隔离与未授权握手，并检查文字在音频帧发送完成前到达。只有首文字确实在音频发完前出现，才构成该样本的实时输出证据；短到不足以产生临时结果的录音可能仅返回定稿。

既有验收为短样本功能验证，没有人工 CER/WER 精度评分、长会话或持续并发容量认证。当前临时识别窗口为 8 秒、LLM 上下文上限为 2048，均不能解释成整场录音的最大时长承诺；配置的两路解码也不能解释成两路满长容量保证。
