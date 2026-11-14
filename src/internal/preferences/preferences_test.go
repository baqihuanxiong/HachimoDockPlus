package preferences

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPersistenceAndFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "preferences.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("load created data directory")
	}
	if err = s.SetTheme("violet"); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Snapshot().Theme != "violet" {
		t.Fatal(loaded, err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
	if s.SetTheme("invalid") == nil || s.Snapshot().Theme != "violet" {
		t.Fatal("invalid theme committed")
	}
	// A filesystem failure must leave the active in-memory preference unchanged.
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, []byte("x"), 0600)
	s.path = filepath.Join(blocker, "preferences.json")
	if s.SetTheme("blue") == nil || s.Snapshot().Theme != "violet" {
		t.Fatal("failed save changed theme")
	}
}
func TestConcurrentChangesAndOwnership(t *testing.T) {
	s, _ := Load(filepath.Join(t.TempDir(), "preferences.json"))
	var wg sync.WaitGroup
	for _, theme := range Themes() {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := s.SetTheme(id); err != nil {
				t.Error(err)
			}
			_ = s.Snapshot()
		}(theme.ID)
	}
	wg.Wait()
	loaded, err := Load(s.path)
	if err != nil || loaded.Snapshot() != s.Snapshot() {
		t.Fatal(loaded, err)
	}
	palette := Themes()
	palette[0].ID = "changed"
	if Themes()[0].ID != "teal" {
		t.Fatal("mutable palette escaped")
	}
}

func TestModeMigrationAndIndependentSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	if err := os.WriteFile(path, []byte(`{"theme":"amber"}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Mode != "dark" || s.Snapshot().Theme != "amber" {
		t.Fatal(s.Snapshot())
	}
	if err = s.SetMode("light"); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Theme != "amber" {
		t.Fatal("mode change lost theme")
	}
	if err = s.SetTheme("orange"); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Mode != "light" {
		t.Fatal("theme change lost mode")
	}
	loaded, err := Load(path)
	if err != nil || loaded.Snapshot() != s.Snapshot() {
		t.Fatal(loaded, err)
	}
	if s.SetMode("invalid") == nil || s.Snapshot().Mode != "light" {
		t.Fatal("invalid mode committed")
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := s.SetMode("dark"); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := s.SetTheme("green"); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
	if got := s.Snapshot(); got.Mode != "dark" || got.Theme != "green" {
		t.Fatal("concurrent settings lost", got)
	}
}

func TestScreensaverMigrationAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	if err := os.WriteFile(path, []byte(`{"theme":"teal","mode":"dark"}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Screensaver != "clock" {
		t.Fatal(s.Snapshot())
	}
	if err = s.SetScreensaver("blank"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetMode("light"); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Snapshot().Screensaver != "blank" || loaded.Snapshot().Mode != "light" {
		t.Fatal(loaded, err)
	}
	if s.SetScreensaver("pet") == nil || s.Snapshot().Screensaver != "blank" {
		t.Fatal("invalid saver committed")
	}
}
