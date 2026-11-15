package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
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
	defer desktop.Close()
	s := desktop.Host.Snapshot()
	if s.Active != "" || len(s.Applications) != 4 {
		t.Fatal(s)
	}
	if _, err = os.Stat(filepath.Join(dir, "data")); !os.IsNotExist(err) {
		t.Fatal("catalog initialized app storage")
	}
	if err = desktop.Host.Open("settings"); err != nil {
		t.Fatal(err)
	}
	if desktop.Host.Snapshot().Active != "settings" {
		t.Fatal("settings unavailable without model credentials")
	}
	desktop.Host.Back()
	if err = desktop.Host.Open("picoclaw"); err == nil {
		t.Fatal("missing app settings accepted")
	}
	if desktop.Host.Snapshot().Active != "" {
		t.Fatal("failed app replaced catalog")
	}
}

func TestConfiguredCatalogOrderUsesRegisteredIDsWithoutServices(t *testing.T) {
	for _, tc := range []struct {
		order []string
		want  []string
	}{
		{nil, []string{"picoclaw", "settings", "music", "newapi-monitor"}},
		{[]string{"music"}, []string{"music", "picoclaw", "settings", "newapi-monitor"}},
		{[]string{"settings", "music", "picoclaw"}, []string{"settings", "music", "picoclaw", "newapi-monitor"}},
		{[]string{"newapi-monitor"}, []string{"newapi-monitor", "picoclaw", "settings", "music"}},
		{[]string{"not-installed"}, nil},
		{[]string{"music", "music"}, nil},
	} {
		cfg, err := config.Parse([]byte(`{"data_dir":"data"}`), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		cfg.UI.AppOrder = tc.order
		r, err := applicationRegistry(cfg, nil, nil)
		if tc.want == nil {
			if err == nil {
				t.Fatal("invalid configured order accepted", tc.order)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, m := range r.Manifests() {
			got = append(got, m.ID)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatal(got, tc.want)
		}
	}
}
