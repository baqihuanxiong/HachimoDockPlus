package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeyPermissionsAndNoSecretInErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "key")
	secret := "credential-never-log"
	if err := os.WriteFile(p, []byte(secret), 0644); err != nil {
		t.Fatal(err)
	}
	s := Service{KeyFile: p}
	if _, err := s.Key(); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatal("insecure credential accepted or disclosed", err)
	}
	os.Chmod(p, 0600)
	if key, err := s.Key(); key != secret || err != nil {
		t.Fatal(err)
	}
}
func TestCatalogConfigDoesNotRequireApplicationSecrets(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "config.json")
	os.WriteFile(p, []byte(`{"data_dir":"data","ui":{"font":"font.ttc"},"apps":{}}`), 0600)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.DataDir != filepath.Join(root, "data") || c.UI.Font != filepath.Join(root, "font.ttc") || c.AppDataDir("test") != filepath.Join(root, "data/apps/test") {
		t.Fatal("incorrect paths")
	}
	if err = c.DecodeApp("missing", new(any)); err == nil {
		t.Fatal("missing app accepted")
	}
}
func TestLegacyConfigPreservesExistingDataDirectory(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "config.json")
	os.WriteFile(p, []byte(`{"data_dir":"data","llm":{"model":"model"},"asr":{},"tts":{}}`), 0600)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	var legacy struct {
		DataDir       string `json:"data_dir"`
		LLM, ASR, TTS any
	}
	if err = c.DecodeApp("picoclaw", &legacy); err != nil {
		t.Fatal(err)
	}
	if c.Resolve(legacy.DataDir) != c.DataDir {
		t.Fatal("legacy workspace would be moved")
	}
}
func TestServiceURLRejectsEmbeddedCredentials(t *testing.T) {
	for _, u := range []string{"wss://asr.example?key=secret", "wss://user:secret@asr.example", "wss://asr.example#secret"} {
		if err := (Service{URL: u, KeyFile: "key"}).Validate("asr", true); err == nil {
			t.Fatal("unsafe URL accepted")
		}
	}
}
