package speech

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func reference(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "reference.wav")
	b := make([]byte, 100)
	copy(b, "RIFF")
	copy(b[8:], "WAVE")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTTSNativeMultipartAndAlignedStreaming(t *testing.T) {
	expected := []byte{1, 2, 3, 4, 5, 6}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/inference_instruct2" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer key" {
			t.Error("invalid native request")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			return
		}
		defer r.MultipartForm.RemoveAll()
		if r.FormValue("tts_text") != "你好" || r.FormValue("stream") != "true" || r.FormValue("instruct_text") != "You are a helpful assistant. 自然<|endofprompt|>" {
			t.Error("bad CosyVoice fields")
		}
		f, _, err := r.FormFile("prompt_wav")
		if err != nil {
			t.Error("reference wasn't uploaded")
		} else {
			b, _ := io.ReadAll(f)
			f.Close()
			if len(b) != 100 {
				t.Error("wrong reference bytes")
			}
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		for _, chunk := range [][]byte{{1}, {2, 3}, {4}, {5, 6}} {
			w.Write(chunk)
			w.(http.Flusher).Flush()
		}
	}))
	defer s.Close()
	tts := TTS{URL: s.URL, Key: "key", Mode: "instruct2", PromptWAV: reference(t), Instruction: "自然"}
	pcm, err := tts.Synthesize(context.Background(), "你好")
	if err != nil {
		t.Fatal(err)
	}
	defer pcm.Close()
	var out bytes.Buffer
	buf := make([]byte, 4)
	for {
		n, e := pcm.Read(buf)
		if n%2 != 0 {
			t.Fatal("unaligned PCM")
		}
		out.Write(buf[:n])
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
	}
	if !bytes.Equal(out.Bytes(), expected) {
		t.Fatal(out.Bytes())
	}
}

func TestTTSRejectsTruncatedAndEmptyPCM(t *testing.T) {
	for _, payload := range [][]byte{nil, {1, 2, 3}} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(payload)
		}))
		tts := TTS{URL: s.URL, Key: "key", Mode: "cross_lingual", PromptWAV: reference(t)}
		r, err := tts.Synthesize(context.Background(), "hello")
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.ReadAll(r)
		r.Close()
		s.Close()
		if err == nil {
			t.Fatal("incomplete PCM accepted")
		}
	}
}

func TestTTSErrorBodyIsNotExposed(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte("private server diagnostic"))
	}))
	defer s.Close()
	tts := TTS{URL: s.URL, Key: "key", Mode: "instruct2", PromptWAV: reference(t)}
	_, err := tts.Synthesize(context.Background(), "x")
	if err == nil || err.Error() != "TTS 请求失败（HTTP 401）" {
		t.Fatal(err)
	}
}
