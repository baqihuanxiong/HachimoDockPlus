package speech

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
)

type TTS struct {
	URL, Key, Mode, PromptWAV, PromptText, Instruction string
	Provider, ResourceID, Speaker                      string
	SpeechRate                                         int
	Client                                             *http.Client
}

func (t TTS) Ready() error {
	if t.Provider == ProviderVolcengine {
		if t.Key == "" || t.Speaker == "" || t.ResourceID == "" {
			return errors.New("请配置豆包 TTS 密钥、资源 ID 和音色")
		}
		return nil
	}
	s, err := os.Stat(t.PromptWAV)
	if err != nil || !s.Mode().IsRegular() || s.Size() < 44 || s.Size() > 4<<20 {
		return errors.New("请配置有效的 TTS 参考 WAV 音频（最大 4 MiB）")
	}
	return nil
}

// Synthesize returns streaming, sample-aligned 24kHz PCM, never a WAV file.
func (t TTS) Synthesize(ctx context.Context, text string) (io.ReadCloser, error) {
	if t.Provider == ProviderVolcengine {
		return t.synthesizeVolcengine(ctx, text)
	}
	if err := t.Ready(); err != nil {
		return nil, err
	}
	file, err := os.Open(t.PromptWAV)
	if err != nil {
		return nil, errors.New("无法打开 TTS 参考音频")
	}
	defer file.Close()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fields := map[string]string{"tts_text": text, "stream": "true"}
	prefix, marker := "You are a helpful assistant.", "<|endofprompt|>"
	switch t.Mode {
	case "zero_shot":
		fields["prompt_text"] = prefix + marker + t.PromptText
	case "cross_lingual":
		fields["tts_text"] = prefix + marker + text
	case "instruct2":
		fields["instruct_text"] = prefix + " " + t.Instruction + marker
	default:
		return nil, errors.New("不支持的 TTS 模式")
	}
	for k, v := range fields {
		if err = w.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	part, err := w.CreateFormFile("prompt_wav", "reference.wav")
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(part, io.LimitReader(file, 4<<20)); err != nil {
		return nil, err
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(t.URL, "/")+"/inference_"+t.Mode, &body)
	if err != nil {
		return nil, errors.New("TTS 请求地址无效")
	}
	req.Header.Set("Authorization", "Bearer "+t.Key)
	req.Header.Set("Content-Type", w.FormDataContentType())
	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("TTS 服务连接失败")
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("TTS 请求失败（HTTP %d）", resp.StatusCode)
	}
	if strings.Split(resp.Header.Get("Content-Type"), ";")[0] != "application/octet-stream" {
		resp.Body.Close()
		return nil, errors.New("TTS 返回了非 PCM 响应")
	}
	return &pcmBody{ReadCloser: resp.Body, reader: bufio.NewReader(resp.Body)}, nil
}

type pcmBody struct {
	io.ReadCloser
	reader   *bufio.Reader
	bytes    int
	terminal error
}

func (p *pcmBody) Read(b []byte) (int, error) {
	if p.terminal != nil {
		return 0, p.terminal
	}
	if len(b) == 0 {
		return 0, nil
	}
	if len(b) < 2 {
		return 0, errors.New("PCM 缓冲区至少需要两个字节")
	}
	next := min(len(b)&^1, 1920) // <=40ms audio; no full-response buffering.
	n, err := io.ReadFull(p.reader, b[:next])
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		if n%2 != 0 {
			p.terminal = errors.New("TTS PCM 响应未对齐，音频不完整")
			return n &^ 1, p.terminal
		}
		if p.bytes+n == 0 {
			return 0, errors.New("TTS 返回空音频")
		}
		p.terminal = io.EOF
		if n > 0 {
			err = nil
		}
	}
	if err != nil && err != io.EOF {
		p.terminal = err
		n &= ^1
	}
	p.bytes += n
	return n, err
}
