package system

import (
	"context"
	"encoding/json"
	"errors"
	adapter "hachimodockplus/internal/agent/picoclaw"
	"hachimodockplus/internal/agent/profile"
	"os"
	"path/filepath"
	"testing"
)

func configurationFixture(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	if err := os.Mkdir(filepath.Join(base, "secrets"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "config.json"), []byte(`{"model":"old"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "secrets", "model.key"), []byte("old-key"), 0600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(base, "config.json")
}
func readTest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func validateFixture(data []byte, secrets string) error {
	var cfg struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(data, &cfg) != nil || cfg.Model == "" {
		return errors.New("invalid model")
	}
	key, err := os.ReadFile(filepath.Join(secrets, "model.key"))
	if err != nil || len(key) == 0 {
		return errors.New("missing key")
	}
	return nil
}
func TestPrepareIsIdempotentAndKeepsPendingEdits(t *testing.T) {
	path := configurationFixture(t)
	base := filepath.Dir(path)
	if err := PrepareConfiguration(path); err != nil {
		t.Fatal(err)
	}
	if readTest(t, path) != `{"model":"old"}` || readTest(t, filepath.Join(base, "secrets", "model.key")) != "old-key" {
		t.Fatal("migration changed live configuration")
	}
	edit := filepath.Join(base, "config-share", "config.json")
	if err := os.WriteFile(edit, []byte(`{"model":"pending"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := PrepareConfiguration(path); err != nil {
		t.Fatal(err)
	}
	if readTest(t, edit) != `{"model":"pending"}` || readTest(t, path) != `{"model":"old"}` {
		t.Fatal("reinstall replaced pending or active data")
	}
}
func TestApplySwitchesJSONAndKeysAndPrunesOldGeneration(t *testing.T) {
	path := configurationFixture(t)
	base := filepath.Dir(path)
	if err := PrepareConfiguration(path); err != nil {
		t.Fatal(err)
	}
	share := filepath.Join(base, "config-share")
	os.WriteFile(filepath.Join(share, "config.json"), []byte(`{"model":"new"}`), 0600)
	os.WriteFile(filepath.Join(share, "secrets", "model.key"), []byte("new-key"), 0600)
	if err := ApplyConfiguration(context.Background(), path, validateFixture); err != nil {
		t.Fatal(err)
	}
	if readTest(t, path) != `{"model":"new"}` || readTest(t, filepath.Join(base, "secrets", "model.key")) != "new-key" {
		t.Fatal("JSON and credentials were not switched together")
	}
	entries, _ := os.ReadDir(filepath.Join(base, "system", "config-versions"))
	if len(entries) != 1 {
		t.Fatal("old configuration backups retained", len(entries))
	}
	st, _ := os.Stat(filepath.Join(base, "secrets", "model.key"))
	if st.Mode().Perm() != 0600 {
		t.Fatal("key permissions changed")
	}
}
func TestInvalidOrChangingCandidateNeverReplacesActive(t *testing.T) {
	for _, kind := range []string{"invalid-json", "missing-key", "symlink", "editing", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			path := configurationFixture(t)
			base := filepath.Dir(path)
			if err := PrepareConfiguration(path); err != nil {
				t.Fatal(err)
			}
			share := filepath.Join(base, "config-share")
			key := filepath.Join(share, "secrets", "model.key")
			ctx := context.Background()
			validate := Validator(validateFixture)
			switch kind {
			case "invalid-json":
				os.WriteFile(filepath.Join(share, "config.json"), []byte("{"), 0600)
			case "missing-key":
				os.Remove(key)
			case "symlink":
				os.Remove(key)
				os.Symlink(filepath.Join(base, "secrets", "model.key"), key)
			case "editing":
				validate = func(data []byte, secrets string) error {
					os.WriteFile(key, []byte("changed-during-validation"), 0600)
					return validateFixture(data, secrets)
				}
			case "canceled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			if err := ApplyConfiguration(ctx, path, validate); err == nil {
				t.Fatal("invalid candidate accepted")
			}
			if readTest(t, path) != `{"model":"old"}` || readTest(t, filepath.Join(base, "secrets", "model.key")) != "old-key" {
				t.Fatal("live configuration damaged")
			}
			entries, _ := os.ReadDir(filepath.Join(base, "system", "config-versions"))
			if len(entries) != 1 {
				t.Fatal("failed staging directory leaked")
			}
		})
	}
}

func TestAgentAssetsSwitchAtomicallyAndKeepExecutionPermissions(t *testing.T) {
	path := configurationFixture(t)
	base := filepath.Dir(path)
	if err := PrepareConfiguration(path); err != nil {
		t.Fatal(err)
	}
	share := filepath.Join(base, "config-share/agent")
	assets, err := profile.Read(share)
	if err != nil {
		t.Fatal(err)
	}
	assets["bin/custom-cli"] = []byte("#!/bin/sh\necho uploaded\n")
	assets["skills/custom/SKILL.md"] = []byte("---\nname: custom\ndescription: Run custom-cli\n---\nUse custom-cli.\n")
	if err := profile.Write(share, assets); err != nil {
		t.Fatal(err)
	}
	os.Chmod(filepath.Join(share, "bin/custom-cli"), 0600)
	validate := func(data []byte, secrets string) error {
		return adapter.ValidateExtensions(filepath.Join(filepath.Dir(secrets), "agent"), base)
	}
	if err := ApplyConfiguration(context.Background(), path, validate); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(base, "agent/bin/custom-cli"))
	if err != nil || st.Mode().Perm() != 0700 {
		t.Fatal("CLI not executable", err)
	}
	// Invalid runtime JSON cannot replace a working profile.
	os.WriteFile(filepath.Join(share, "runtime.json"), []byte("{"), 0600)
	if err := ApplyConfiguration(context.Background(), path, validate); err == nil {
		t.Fatal("invalid agent config accepted")
	}
	if err := adapter.ValidateExtensions(filepath.Join(base, "agent"), base); err != nil {
		t.Fatal("active profile damaged", err)
	}
	assets["runtime.json"] = profile.Defaults()["runtime.json"]
	if err := profile.Write(share, assets); err != nil {
		t.Fatal(err)
	}
	// Uploading while validation runs invalidates the entire snapshot.
	if err := ApplyConfiguration(context.Background(), path, func(data []byte, secrets string) error {
		os.WriteFile(filepath.Join(share, "bin/custom-cli"), []byte("changed"), 0600)
		return validate(data, secrets)
	}); err == nil {
		t.Fatal("concurrent CLI edit accepted")
	}
	if got := readTest(t, filepath.Join(base, "agent/bin/custom-cli")); got != string(assets["bin/custom-cli"]) {
		t.Fatal("unapplied CLI edit became live")
	}
}

func TestAgentSnapshotRejectsSymlinksAndPreservesPendingEditsOnUpgrade(t *testing.T) {
	path := configurationFixture(t)
	base := filepath.Dir(path)
	if err := PrepareConfiguration(path); err != nil {
		t.Fatal(err)
	}
	share := filepath.Join(base, "config-share/agent")
	os.WriteFile(filepath.Join(share, "AGENT.md"), []byte("pending user edits"), 0600)
	if err := PrepareConfiguration(path); err != nil {
		t.Fatal(err)
	}
	if readTest(t, filepath.Join(share, "AGENT.md")) != "pending user edits" {
		t.Fatal("pending edits replaced")
	}
	os.Symlink(filepath.Join(base, "secrets"), filepath.Join(share, "escape"))
	if err := ApplyConfiguration(context.Background(), path, validateFixture); err == nil {
		t.Fatal("agent symlink followed")
	}
}

func TestAssetsAreStagedValidatedAndSwitchedWithConfig(t *testing.T) {
	path := configurationFixture(t)
	base := filepath.Dir(path)
	os.Mkdir(filepath.Join(base, "assets"), 0700)
	os.WriteFile(filepath.Join(base, "assets", "reference.wav"), []byte("old voice"), 0600)
	if err := PrepareConfiguration(path); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(base, "assets", "reference.wav")
	share := filepath.Join(base, "config-share", "assets", "reference.wav")
	if readTest(t, share) != "old voice" {
		t.Fatal("assets not shared")
	}
	os.WriteFile(share, []byte("pending voice"), 0600)
	if err := PrepareConfiguration(path); err != nil {
		t.Fatal(err)
	}
	if readTest(t, live) != "old voice" || readTest(t, share) != "pending voice" {
		t.Fatal("upgrade applied or replaced pending voice")
	}
	reject := func(data []byte, secrets string) error {
		candidate := CandidatePath(base, filepath.Dir(secrets), live)
		if readTest(t, candidate) != "pending voice" {
			t.Fatal("validator saw live voice")
		}
		return errors.New("reject candidate")
	}
	if err := ApplyConfiguration(context.Background(), path, reject); err == nil {
		t.Fatal("invalid candidate accepted")
	}
	if readTest(t, live) != "old voice" {
		t.Fatal("unapplied voice became live")
	}
	if err := ApplyConfiguration(context.Background(), path, func(data []byte, secrets string) error {
		if err := validateFixture(data, secrets); err != nil {
			return err
		}
		os.WriteFile(share, []byte("racing edit"), 0600)
		return nil
	}); err == nil {
		t.Fatal("concurrent resource edit accepted")
	}
	os.WriteFile(share, []byte("final voice"), 0600)
	if err := ApplyConfiguration(context.Background(), path, validateFixture); err != nil {
		t.Fatal(err)
	}
	if readTest(t, live) != "final voice" {
		t.Fatal("voice did not switch")
	}
	os.Remove(share)
	os.Symlink(live, share)
	if err := ApplyConfiguration(context.Background(), path, validateFixture); err == nil {
		t.Fatal("asset symlink accepted")
	}
}
