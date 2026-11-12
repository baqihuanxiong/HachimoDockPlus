package speech

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type ASR struct{ URL, Key, Language string }

type Transcript struct {
	Event     string `json:"event"`
	Sentences []struct {
		Text string `json:"text"`
	} `json:"sentences"`
	Partial string `json:"partial"`
	Final   bool   `json:"is_final"`
}

func (r Transcript) Text(preview bool) string {
	var s strings.Builder
	for _, sentence := range r.Sentences {
		s.WriteString(sentence.Text)
	}
	if preview {
		s.WriteString(r.Partial)
	}
	return strings.TrimSpace(s.String())
}

// Transcribe consumes live PCM16 LE, mono, 16kHz. The caller closes/stops its
// capture to signal EOF; only a confirmed final snapshot may reach the agent.
func (a ASR) Transcribe(ctx context.Context, pcm io.Reader, update func(string)) (string, error) {
	// A live capture must be interrupted even if the server closes early.
	if closer, ok := pcm.(io.Closer); ok {
		defer closer.Close()
		stopReader := context.AfterFunc(ctx, func() { _ = closer.Close() })
		defer stopReader()
	}
	d := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
	conn, response, err := d.DialContext(ctx, a.URL, http.Header{"Authorization": {"Bearer " + a.Key}})
	if err != nil {
		if response != nil {
			response.Body.Close()
			return "", fmt.Errorf("ASR 连接失败（HTTP %d）", response.StatusCode)
		}
		return "", errors.New("ASR 服务连接失败")
	}
	defer conn.Close()
	stopCancel := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopCancel()
	conn.SetReadLimit(1 << 20)
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
	}
	writeText := func(s string) error {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteMessage(websocket.TextMessage, []byte(s))
	}
	expect := func(name string) error {
		var result Transcript
		if err := conn.ReadJSON(&result); err != nil {
			return errors.New("ASR 控制确认失败")
		}
		if result.Event != name {
			return errors.New("ASR 控制事件不符合协议")
		}
		return nil
	}
	if err = writeText("START"); err != nil {
		return "", errors.New("ASR START 发送失败")
	}
	if err = expect("started"); err != nil {
		return "", err
	}
	if err = writeText("LANGUAGE:" + a.Language); err != nil {
		return "", errors.New("ASR LANGUAGE 发送失败")
	}
	if err = expect("language_set"); err != nil {
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
		buf := make([]byte, 3200)
		for {
			n, readErr := io.ReadFull(pcm, buf)
			if n%2 != 0 {
				sendErr = errors.New("ASR PCM 样本未对齐")
				return
			}
			if n > 0 {
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
					sendErr = errors.New("ASR 音频发送失败")
					return
				}
			}
			if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
				sendErr = writeText("STOP")
				return
			}
			if readErr != nil {
				sendErr = errors.New("录音流读取失败")
				return
			}
		}
	}()
	var final *Transcript
	for {
		var result Transcript
		if err := conn.ReadJSON(&result); err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			select {
			case err := <-sent:
				if err != nil {
					return "", err
				}
			default:
			}
			return "", errors.New("ASR 连接中断，未取得完整识别结果")
		}
		if result.Event == "error" {
			return "", errors.New("ASR 服务拒绝了请求")
		}
		if result.Event == "stopped" {
			if final == nil {
				return "", errors.New("ASR 未返回确认文本，请重试")
			}
			select {
			case err := <-sent:
				if err != nil {
					return "", err
				}
			case <-ctx.Done():
				return "", ctx.Err()
			}
			return final.Text(false), nil
		}
		if result.Sentences != nil {
			if update != nil {
				update(result.Text(true))
			}
			if result.Final {
				copy := result
				final = &copy
			}
		}
	}
}
