package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"hachimodockplus/internal/config"
)

func TestCatalogDoesNotInitializePicoClaw(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"data_dir":"data","apps":{}}`), 0600)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	desktop, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer desktop.Host.Close()
	s := desktop.Host.Snapshot()
	if s.Active != "" || len(s.Applications) != 1 {
		t.Fatal(s)
	}
	if _, err = os.Stat(filepath.Join(dir, "data")); !os.IsNotExist(err) {
		t.Fatal("catalog initialized app storage")
	}
	if err = desktop.Host.Open("picoclaw"); err == nil {
		t.Fatal("missing app settings accepted")
	}
	if desktop.Host.Snapshot().Active != "" {
		t.Fatal("failed app replaced catalog")
	}
}
