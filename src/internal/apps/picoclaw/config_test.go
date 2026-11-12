package picoclaw

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hachimodockplus/internal/config"
)

func TestAppConfigPathsValidationAndLegacyWorkspace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	services := `"llm":{"url":"https://llm.example/v1","model":"model","key_file":"llm.key"},"asr":{"url":"wss://asr.example","key_file":"asr.key"},"tts":{"url":"https://tts.example","key_file":"tts.key","prompt_wav":"voice.wav"}`
	for _, legacy := range []bool{false, true} {
		body := `{"data_dir":"data","apps":{"picoclaw":{` + services + `}}}`
		if legacy {
			body = `{"data_dir":"data",` + services + `}`
		}
		os.WriteFile(path, []byte(body), 0600)
		root, err := config.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		c, err := LoadConfig(root)
		if err != nil {
			t.Fatal(err)
		}
		expected := filepath.Join(dir, "data/apps/picoclaw")
		if legacy {
			expected = filepath.Join(dir, "data")
		}
		if c.DataDir != expected || c.TTS.PromptWAV != filepath.Join(dir, "voice.wav") {
			t.Fatal("incorrect app paths", c.DataDir)
		}
		os.WriteFile(path, []byte(strings.Replace(body, "wss://asr.example", "wss://asr.example?key=secret", 1)), 0600)
		root, err = config.Load(path)
		if err != nil {
			t.Fatal("shell should not decode app settings", err)
		}
		if _, err = LoadConfig(root); err == nil {
			t.Fatal("app accepted credential URL")
		}
	}
}
