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

func TestVolcengineConfigDefaultsAndMixedProviders(t *testing.T) {
	for _, asr := range []string{`{"provider":"volcengine","key_file":"speech.key"}`, `{"url":"wss://asr.example","key_file":"asr.key"}`} {
		raw := `{"data_dir":"data","apps":{"picoclaw":{"llm":{"url":"https://llm.example/v1","model":"model","key_file":"llm.key"},"asr":` + asr + `,"tts":{"provider":"volcengine","key_file":"speech.key"}}}}`
		root, err := config.Parse([]byte(raw), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadConfig(root)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.TTS.Speaker != "zh_female_vv_uranus_bigtts" || cfg.TTS.ResourceID != "seed-tts-2.0" || cfg.TTS.PromptWAV != "" {
			t.Fatal("bad Doubao defaults")
		}
		if cfg.ASR.Provider == "volcengine" && (cfg.ASR.ResourceID != "volc.seedasr.sauc.duration" || !strings.HasPrefix(cfg.ASR.URL, "wss://openspeech.bytedance.com/")) {
			t.Fatal("bad ASR defaults")
		}
		for _, bad := range []string{strings.Replace(raw, `"provider":"volcengine"`, `"provider":"invalid"`, 1), strings.Replace(raw, `"tts":{`, `"tts":{"speech_rate":101,`, 1), strings.Replace(raw, `"tts":{`, `"tts":{"resource_id":"bad\r\nheader",`, 1)} {
			r, e := config.Parse([]byte(bad), t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			if _, e = LoadConfig(r); e == nil {
				t.Fatal("bad provider configuration accepted")
			}
		}
	}
}
