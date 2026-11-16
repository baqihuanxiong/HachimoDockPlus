package speech

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

var speechHTTP = &http.Client{Timeout: 60 * time.Second}

func (t TTS) synthesizeVolcengine(ctx context.Context, text string) (io.ReadCloser, error) {
	if err := t.Ready(); err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{
		"user":       map[string]string{"uid": "hachimodockplus"},
		"req_params": map[string]any{"text": text, "speaker": t.Speaker, "audio_params": map[string]any{"format": "pcm", "sample_rate": 24000, "speech_rate": t.SpeechRate}},
	})
	if err != nil {
		return nil, errors.New("豆包 TTS 请求编码失败")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.URL, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("豆包 TTS 请求地址无效")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", t.Key)
	req.Header.Set("X-Api-Resource-Id", t.ResourceID)
	req.Header.Set("X-Api-Request-Id", uuid.NewString())
	client := t.Client
	if client == nil {
		client = speechHTTP
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("豆包 TTS 服务连接失败")
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("豆包 TTS 请求失败（HTTP %d）", resp.StatusCode)
	}
	// Pipe applies backpressure and propagates failures after audio has started.
	reader, writer := io.Pipe()
	go func() {
		defer resp.Body.Close()
		writer.CloseWithError(decodeTTSStream(resp.Body, writer))
	}()
	return &pcmBody{ReadCloser: &ttsStream{PipeReader: reader, body: resp.Body}, reader: bufio.NewReader(reader)}, nil
}

type ttsStream struct {
	*io.PipeReader
	body io.Closer
}

func (s *ttsStream) Close() error { _ = s.PipeReader.Close(); return s.body.Close() }

func decodeTTSStream(r io.Reader, out io.Writer) error {
	limited := &io.LimitedReader{R: r, N: maxSpeechPacket}
	decoder := json.NewDecoder(limited)
	total := 0
	for {
		// Bound each JSON packet while allowing a long, incrementally consumed stream.
		limited.N = maxSpeechPacket
		var packet struct {
			Code *int64 `json:"code"`
			Data string `json:"data"`
		}
		if err := decoder.Decode(&packet); err != nil {
			return errors.New("豆包 TTS 流式响应无效或未完整结束")
		}
		if packet.Code == nil {
			return errors.New("豆包 TTS 响应缺少状态码")
		}
		if *packet.Code == 20000000 {
			if total == 0 {
				return errors.New("豆包 TTS 返回空音频")
			}
			if total%2 != 0 {
				return errors.New("豆包 TTS PCM 音频未对齐")
			}
			return nil
		}
		if *packet.Code != 0 {
			return fmt.Errorf("豆包 TTS 合成失败（%d）", *packet.Code)
		}
		pcm, err := base64.StdEncoding.DecodeString(packet.Data)
		if err != nil {
			return errors.New("豆包 TTS 音频编码无效")
		}
		total += len(pcm)
		if total > 32<<20 {
			return errors.New("豆包 TTS 音频超过长度限制")
		}
		if len(pcm) > 0 {
			if _, err := out.Write(pcm); err != nil {
				return err
			}
		}
	}
}
