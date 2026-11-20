package config

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestCatalogOrderJSON(t *testing.T) {
	c, err := Parse([]byte(`{"data_dir":"data","ui":{"app_order":["music","picoclaw"]}}`), t.TempDir())
	if err != nil || !reflect.DeepEqual(c.UI.AppOrder, []string{"music", "picoclaw"}) {
		t.Fatal(c.UI.AppOrder, err)
	}
	if _, err = Parse([]byte(`{"data_dir":"data","ui":{"app_order":"music"}}`), t.TempDir()); err == nil {
		t.Fatal("non-array app order accepted")
	}
}

func TestPetConfigurationKeepsInstalledAssetBase(t *testing.T) {
	base := t.TempDir()
	c, err := Parse([]byte(`{"data_dir":"data","ui":{"pet":{"directory":"assets/pets/my-pet","wifi_enabled":true}}}`), base)
	if err != nil || !c.UI.Pet.WiFiEnabled || c.UI.Pet.Directory != filepath.Join(base, "assets/pets/my-pet") {
		t.Fatal(c.UI.Pet, err)
	}
	c, err = Parse([]byte(`{"data_dir":"data"}`), base)
	if err != nil || c.UI.Pet.WiFiEnabled || c.UI.Pet.Directory != filepath.Join(base, "assets/pets/default") {
		t.Fatal(c.UI.Pet, err)
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

func TestCandidateAndSymlinkKeepInstalledRelativePaths(t *testing.T) {
	root := t.TempDir()
	data := []byte(`{"data_dir":"data","ui":{"font":"assets/font.ttc"},"apps":{}}`)
	c, err := Parse(data, root)
	if err != nil || c.UI.Font != filepath.Join(root, "assets/font.ttc") || c.DataDir != filepath.Join(root, "data") {
		t.Fatal(c, err)
	}
	physical := filepath.Join(root, "system", "generation")
	os.MkdirAll(physical, 0700)
	os.WriteFile(filepath.Join(physical, "config.json"), data, 0600)
	path := filepath.Join(root, "config.json")
	os.Symlink("system/generation/config.json", path)
	c, err = Load(path)
	if err != nil || c.UI.Font != filepath.Join(root, "assets/font.ttc") || c.SourcePath() != path {
		t.Fatal("generation changed the config's path base", c, err)
	}
}

func TestAudioDefaultsAndValidation(t *testing.T) {
	for _, audio := range []string{`{}`, `{"volume":0,"output":"speaker"}`, `{"volume":100,"output":"spdif"}`, `{"output":"bluetooth","bluetooth_device":"AA:BB:CC:DD:EE:FF"}`} {
		c, err := Parse([]byte(`{"data_dir":"data","audio":`+audio+`}`), t.TempDir())
		if err != nil {
			t.Fatal(audio, err)
		}
		if audio == `{}` && (c.Audio.Volume != 60 || c.Audio.Output != "speaker" || c.Audio.SPDIFDevice == "") {
			t.Fatal(c.Audio)
		}
	}
	for _, audio := range []string{`{"volume":-1}`, `{"volume":101}`, `{"volume":1.5}`, `{"output":"invalid"}`, `{"output":"bluetooth"}`, `{"bluetooth_device":"bad"}`} {
		if _, err := Parse([]byte(`{"data_dir":"data","audio":`+audio+`}`), t.TempDir()); err == nil {
			t.Fatal("invalid audio accepted", audio)
		}
	}
}

func TestAudioMigrationAndMergePreserveCandidateAndStableLink(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "data"), 0700)
	os.MkdirAll(filepath.Join(root, "system"), 0700)
	os.MkdirAll(filepath.Join(root, "config-share"), 0700)
	active := filepath.Join(root, "system", "active.json")
	path := filepath.Join(root, "config.json")
	os.WriteFile(active, []byte(`{"data_dir":"data","audio":{"playback_device":"custom"},"apps":{"example":{"custom":true}}}`), 0600)
	os.Symlink("system/active.json", path)
	share := filepath.Join(root, "config-share", "config.json")
	os.WriteFile(share, []byte(`{"data_dir":"data","audio":{"volume":12,"output":"spdif"},"apps":{"example":{"custom":"pending"}}}`), 0600)
	os.WriteFile(filepath.Join(root, "data", "volume.json"), []byte(`{"volume":37}`), 0600)
	c, err := Load(path)
	if err != nil || c.Audio.Volume != 37 {
		t.Fatal(c.Audio, err)
	}
	if err = c.InitializeAudio(); err != nil {
		t.Fatal(err)
	}
	candidate, err := Load(share)
	if err != nil || candidate.Audio.Volume != 12 || candidate.Audio.Output != "spdif" {
		t.Fatal(candidate.Audio, err)
	}
	if _, err = os.Stat(filepath.Join(root, "data", "volume.json")); !os.IsNotExist(err) {
		t.Fatal("legacy preference retained")
	}
	if err = c.SaveAudio(45, "speaker", ""); err != nil {
		t.Fatal(err)
	}
	candidate, _ = Load(share)
	if candidate.Audio.Volume != 45 || candidate.Audio.Output != "spdif" {
		t.Fatal("unrelated pending output overwritten", candidate.Audio)
	}
	data, _ := os.ReadFile(share)
	if !strings.Contains(string(data), `"pending"`) {
		t.Fatal("pending app edit lost")
	}
	if _, err = os.Readlink(path); err != nil {
		t.Fatal("stable config link replaced")
	}
	c, err = Load(path)
	if err != nil || c.Audio.Volume != 45 || c.Audio.PlaybackDevice != "custom" {
		t.Fatal(c.Audio, err)
	}
}
