package speech

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	ProviderVolcengine    = "volcengine"
	VolcengineASRURL      = "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async"
	VolcengineASRResource = "volc.seedasr.sauc.duration"
	VolcengineTTSURL      = "https://openspeech.bytedance.com/api/v3/tts/unidirectional"
	VolcengineTTSResource = "seed-tts-2.0"
	VolcengineSpeaker     = "zh_female_vv_uranus_bigtts"
	maxSpeechPacket       = 4 << 20
)

func asrPacket(kind, flags, serialization byte, payload []byte) ([]byte, error) {
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	if _, err := w.Write(payload); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	packet := []byte{0x11, kind<<4 | flags, serialization<<4 | 1, 0}
	packet = binary.BigEndian.AppendUint32(packet, uint32(compressed.Len()))
	return append(packet, compressed.Bytes()...), nil
}

func decodeASRPacket(data []byte) (payload []byte, final bool, err error) {
	invalid := errors.New("豆包 ASR 响应帧无效")
	if len(data) < 8 || len(data) > maxSpeechPacket || data[0]>>4 != 1 {
		return nil, false, invalid
	}
	offset := int(data[0]&15) * 4
	kind, flags := data[1]>>4, data[1]&15
	if offset < 4 || offset > len(data)-4 || (kind != 9 && kind != 15) || flags > 3 || data[2]>>4 > 1 {
		return nil, false, invalid
	}
	final = flags&2 != 0
	if flags&1 != 0 {
		sequence := int32(binary.BigEndian.Uint32(data[offset:]))
		final = final || sequence < 0
		offset += 4
	}
	if offset > len(data)-4 {
		return nil, false, invalid
	}
	if kind == 15 {
		// Do not surface provider text, which may contain credentials or input.
		return nil, false, fmt.Errorf("豆包 ASR 服务错误（%d）", binary.BigEndian.Uint32(data[offset:]))
	}
	size := uint64(binary.BigEndian.Uint32(data[offset:]))
	offset += 4
	if size != uint64(len(data)-offset) {
		return nil, false, invalid
	}
	payload = data[offset:]
	switch data[2] & 15 {
	case 0:
	case 1:
		r, e := gzip.NewReader(bytes.NewReader(payload))
		if e != nil {
			return nil, false, invalid
		}
		defer r.Close()
		payload, e = io.ReadAll(io.LimitReader(r, maxSpeechPacket+1))
		if e != nil || len(payload) > maxSpeechPacket {
			return nil, false, invalid
		}
	default:
		return nil, false, invalid
	}
	return payload, final, nil
}

func (a ASR) transcribeVolcengine(ctx context.Context, pcm io.Reader, update func(string)) (string, error) {
	if closer, ok := pcm.(io.Closer); ok {
		defer closer.Close()
		stop := context.AfterFunc(ctx, func() { _ = closer.Close() })
		defer stop()
	}
	headers := http.Header{
		"X-Api-Key": {a.Key}, "X-Api-Resource-Id": {a.ResourceID},
		"X-Api-Request-Id": {uuid.NewString()}, "X-Api-Connect-Id": {uuid.NewString()}, "X-Api-Sequence": {"-1"},
	}
	d := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
	conn, response, err := d.DialContext(ctx, a.URL, headers)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if response != nil {
			response.Body.Close()
			return "", fmt.Errorf("豆包 ASR 连接失败（HTTP %d）", response.StatusCode)
		}
		return "", errors.New("豆包 ASR 服务连接失败")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	conn.SetReadLimit(maxSpeechPacket)
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
	}
	send := func(kind, flags, serialization byte, data []byte) error {
		frame, err := asrPacket(kind, flags, serialization, data)
		if err != nil {
			return errors.New("豆包 ASR 音频编码失败")
		}
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			return errors.New("豆包 ASR 数据发送失败")
		}
		return nil
	}
	// Match HachimoDock's optimized streaming recognition request.
	init, _ := json.Marshal(map[string]any{
		"user":    map[string]string{"uid": "hachimodockplus"},
		"audio":   map[string]any{"format": "pcm", "codec": "raw", "rate": 16000, "bits": 16, "channel": 1},
		"request": map[string]any{"model_name": "bigmodel", "enable_nonstream": true, "enable_itn": true, "enable_punc": true, "enable_ddc": true, "show_utterances": true},
	})
	if err := send(1, 0, 1, init); err != nil {
		return "", err
	}
	sent := make(chan error, 1)
	go func() {
		var sendErr error
		defer func() {
			sent <- sendErr
			if sendErr != nil {
				conn.Close()
			}
		}()
		// Keep one chunk for the final marker: the last packet contains real PCM.
		var pending []byte
		for {
			buf := make([]byte, 6400) // 200 ms of 16 kHz mono PCM16.
			n, readErr := io.ReadFull(pcm, buf)
			if n%2 != 0 {
				sendErr = errors.New("ASR PCM 样本未对齐")
				return
			}
			if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
				sendErr = errors.New("录音流读取失败")
				return
			}
			if n > 0 {
				if pending != nil {
					if sendErr = send(2, 0, 0, pending); sendErr != nil {
						return
					}
				}
				pending = buf[:n]
			}
			if readErr != nil {
				if len(pending) == 0 {
					pending = make([]byte, 640)
				} // Empty recording still needs a valid final audio packet.
				deadline := time.Now().Add(10 * time.Second)
				if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
					deadline = end
				}
				_ = conn.SetReadDeadline(deadline)
				sendErr = send(2, 2, 0, pending)
				return
			}
		}
	}()
	last := ""
	for {
		kind, data, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			select {
			case sendErr := <-sent:
				if sendErr != nil {
					return "", sendErr
				}
			default:
			}
			return "", errors.New("豆包 ASR 连接中断或超时，未取得完整识别结果")
		}
		if kind != websocket.BinaryMessage {
			return "", errors.New("豆包 ASR 返回了非二进制响应")
		}
		payload, final, err := decodeASRPacket(data)
		if err != nil {
			return "", err
		}
		var result struct {
			Code       json.Number `json:"code"`
			StatusCode json.Number `json:"status_code"`
			Result     struct {
				Text       string `json:"text"`
				Utterances []struct {
					Text string `json:"text"`
				} `json:"utterances"`
			} `json:"result"`
		}
		if len(payload) > 0 {
			if json.Unmarshal(payload, &result) != nil {
				return "", errors.New("豆包 ASR 返回了无效 JSON")
			}
			for _, raw := range []json.Number{result.Code, result.StatusCode} {
				if raw == "" {
					continue
				}
				code, err := raw.Int64()
				if err != nil {
					return "", errors.New("豆包 ASR 状态码无效")
				}
				if code != 0 && code != 1000 && code != 20000000 {
					return "", fmt.Errorf("豆包 ASR 识别失败（%d）", code)
				}
			}
			var text strings.Builder
			for _, u := range result.Result.Utterances {
				text.WriteString(u.Text)
			}
			snapshot := strings.TrimSpace(text.String())
			if snapshot == "" {
				snapshot = strings.TrimSpace(result.Result.Text)
			}
			if snapshot != "" {
				last = snapshot
				if update != nil {
					update(last)
				}
			}
		}
		if final {
			select {
			case err := <-sent:
				if err != nil {
					return "", err
				}
			case <-ctx.Done():
				return "", ctx.Err()
			}
			return last, nil
		}
	}
}
