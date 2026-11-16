package speech

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func cloudASRServer(t *testing.T, finish func(*websocket.Conn)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" || r.Header.Get("X-Api-Resource-Id") != VolcengineASRResource || r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Request-Id") == "" || r.Header.Get("X-Api-Sequence") != "-1" {
			t.Error("invalid ASR authentication")
		}
		c, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer c.Close()
		_, data, e := c.ReadMessage()
		if e != nil {
			return
		}
		// Client/server payload framing is identical apart from the message type.
		if data[1]>>4 != 1 {
			t.Error("missing recognition request")
			return
		}
		data[1] = 0x90
		payload, _, e := decodeASRPacket(data)
		var init map[string]any
		if e != nil || json.Unmarshal(payload, &init) != nil || init["audio"].(map[string]any)["rate"] != float64(16000) {
			t.Error("invalid audio request")
			return
		}
		finish(c)
	}))
}

func TestVolcengineASRStreamingAndFinal(t *testing.T) {
	expected := bytes.Repeat([]byte{1, 2}, 6400)
	s := cloudASRServer(t, func(c *websocket.Conn) {
		var received []byte
		for {
			_, data, err := c.ReadMessage()
			if err != nil {
				return
			}
			final := data[1]&2 != 0
			if data[1]>>4 != 2 {
				t.Error("not audio")
				return
			}
			data[1] = 0x90
			pcm, _, err := decodeASRPacket(data)
			if err != nil {
				t.Error(err)
				return
			}
			received = append(received, pcm...)
			if final {
				if len(pcm) == 0 {
					t.Error("empty final frame")
				}
				break
			}
			p, _ := asrPacket(9, 0, 1, []byte(`{"result":{"text":"你好"}}`))
			c.WriteMessage(websocket.BinaryMessage, p)
		}
		if !bytes.Equal(received, expected) {
			t.Error("PCM corrupted")
		}
		p, _ := asrPacket(9, 3, 1, []byte(`{"code":20000000,"result":{"utterances":[{"text":"你好，"},{"text":"世界。"}]}}`))
		// Sequence field precedes payload size when flag 3 is set.
		p = append(append(append([]byte{}, p[:4]...), 0xff, 0xff, 0xff, 0xff), p[4:]...)
		c.WriteMessage(websocket.BinaryMessage, p)
	})
	defer s.Close()
	var previews []string
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	a := ASR{Provider: ProviderVolcengine, URL: "ws" + strings.TrimPrefix(s.URL, "http"), Key: "test-key", ResourceID: VolcengineASRResource}
	text, err := a.Transcribe(ctx, bytes.NewReader(expected), func(text string) { previews = append(previews, text) })
	if err != nil || text != "你好，世界。" || len(previews) < 2 {
		t.Fatalf("text=%q previews=%v err=%v", text, previews, err)
	}
}

func TestVolcengineASRRejectsPartialDisconnectAndCancelsCapture(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disconnect", true: "cancel"}[cancelled], func(t *testing.T) {
			s := cloudASRServer(t, func(c *websocket.Conn) {
				p, _ := asrPacket(9, 0, 1, []byte(`{"result":{"text":"partial"}}`))
				c.WriteMessage(websocket.BinaryMessage, p)
				if cancelled {
					for {
						if _, _, e := c.ReadMessage(); e != nil {
							return
						}
					}
				}
			})
			defer s.Close()
			reader, writer := io.Pipe()
			defer writer.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
			defer cancel()
			a := ASR{Provider: ProviderVolcengine, URL: "ws" + strings.TrimPrefix(s.URL, "http"), Key: "test-key", ResourceID: VolcengineASRResource}
			text, err := a.Transcribe(ctx, reader, nil)
			if err == nil || text != "" {
				t.Fatal("partial accepted", text, err)
			}
			done := make(chan struct{})
			go func() { reader.Read(make([]byte, 2)); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("capture leaked")
			}
		})
	}
}

func TestVolcengineASRFrameValidation(t *testing.T) {
	p, _ := asrPacket(9, 2, 1, []byte(`{"result":{}}`))
	for _, data := range [][]byte{nil, p[:3], p[:len(p)-1], append(append([]byte{}, p...), 0), {0x11, 0x90, 0x12, 0, 0, 0, 0, 0}, {0x11, 0x93, 0x10, 0, 0, 0, 0, 1}} {
		if _, _, err := decodeASRPacket(data); err == nil {
			t.Fatalf("invalid frame accepted: %x", data)
		}
	}
	p = []byte{0x11, 0xf0, 0x10, 0}
	p = binary.BigEndian.AppendUint32(p, 45000001)
	p = binary.BigEndian.AppendUint32(p, 6)
	p = append(p, "secret"...)
	if _, _, err := decodeASRPacket(p); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
}

func TestVolcengineTTSStreamsAlignedPCMWithoutReference(t *testing.T) {
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "key" || r.Header.Get("X-Api-Resource-Id") != VolcengineTTSResource {
			t.Error("bad TTS headers")
		}
		var req struct {
			Params struct {
				Speaker string `json:"speaker"`
				Audio   struct {
					Rate   int    `json:"sample_rate"`
					Format string `json:"format"`
				} `json:"audio_params"`
			} `json:"req_params"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil || req.Params.Audio.Rate != 24000 || req.Params.Audio.Format != "pcm" || req.Params.Speaker != VolcengineSpeaker {
			t.Error("bad TTS request")
		}
		io.WriteString(w, `{"code":0,"data":"AQ=="}{"code":0,"data":"AgME"}`)
		w.(http.Flusher).Flush()
		<-release
		io.WriteString(w, `{"code":20000000}`)
	}))
	defer s.Close()
	tts := TTS{Provider: ProviderVolcengine, URL: s.URL, Key: "key", ResourceID: VolcengineTTSResource, Speaker: VolcengineSpeaker}
	r, err := tts.Synthesize(context.Background(), "你好")
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	defer r.Close()
	done := make(chan []byte, 1)
	go func() {
		b := make([]byte, 4)
		_, err := io.ReadFull(r, b)
		if err != nil {
			t.Error(err)
		}
		done <- b
	}()
	select {
	case b := <-done:
		if !bytes.Equal(b, []byte{1, 2, 3, 4}) {
			t.Error(b)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("buffered full response")
	}
	close(release)
	if _, err := io.ReadAll(r); err != nil {
		t.Fatal(err)
	}
}

func TestVolcengineTTSErrorAndTruncation(t *testing.T) {
	for _, wire := range []string{`{"code":0,"data":"AQI="}`, `{"code":20000000}`, `{"code":0,"data":"AQ=="}{"code":20000000}`, `{"code":0,"data":"???"}`, `{"code":55000000,"message":"secret"}`, `{"data":"AQI="}`, `not json`} {
		err := decodeTTSStream(strings.NewReader(wire), io.Discard)
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("%s: %v", wire, err)
		}
	}
}

func TestVolcengineTTSCloseInterruptsStream(t *testing.T) {
	ended := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(ended)
	}))
	defer s.Close()
	tts := TTS{Provider: ProviderVolcengine, URL: s.URL, Key: "key", ResourceID: VolcengineTTSResource, Speaker: VolcengineSpeaker}
	r, err := tts.Synthesize(context.Background(), "你好")
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("HTTP body leaked")
	}
}
