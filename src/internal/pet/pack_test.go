package pet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOfflineExportIgnoresDesktopPathsAndFallsBack(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "videos"), 0700)
	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"name":"My dog","families":[{"videoPath":"/private/desktop/file.mp4"}]}`), 0600)
	for _, name := range []string{"idle.playing", "working.typing", "working.thinking"} {
		os.WriteFile(filepath.Join(dir, "videos", name+".mp4"), []byte("clip"), 0600)
	}
	p, err := LoadPack(dir)
	if err != nil || p.Name != "My dog" || len(p.Clips) != 3 {
		t.Fatal(p, err)
	}
	name, path := p.Clip("done", 0)
	if name != "idle.playing" || path != filepath.Join(dir, "videos/idle.playing.mp4") {
		t.Fatal(name, path)
	}
	a, _ := p.Clip("working", 0)
	b, _ := p.Clip("working", 1)
	if a == b {
		t.Fatal("working variants lost")
	}
	os.Symlink(filepath.Join(dir, "videos/working.typing.mp4"), filepath.Join(dir, "videos/error.mp4"))
	if _, err = LoadPack(dir); err == nil {
		t.Fatal("linked video accepted")
	}
}

func TestTouchClipRegionsAlternationAndMissingClips(t *testing.T) {
	p := Pack{Clips: map[string]string{"touch.left": "left", "touch.right": "right", "touch.lick": "lick", "touch.what": "what", "idle.default": "idle"}}
	for _, tc := range []struct {
		x        int
		sequence uint64
		want     string
	}{{0, 0, "touch.left"}, {213, 1, "touch.left"}, {214, 0, "touch.lick"}, {320, 1, "touch.what"}, {426, 2, "touch.lick"}, {427, 1, "touch.right"}, {639, 0, "touch.right"}} {
		family, path := p.TouchClip(tc.x, tc.sequence)
		if family != tc.want || path != p.Clips[tc.want] {
			t.Fatalf("x=%d sequence=%d: %s %s", tc.x, tc.sequence, family, path)
		}
	}
	delete(p.Clips, "touch.left")
	delete(p.Clips, "touch.what")
	if family, _ := p.TouchClip(0, 1); family != "touch.lick" {
		t.Fatal("missing directional/alternate clip did not fall back to available touch", family)
	}
	delete(p.Clips, "touch.lick")
	if family, path := p.TouchClip(320, 0); family != "" || path != "" {
		t.Fatal("missing touch must not play an unrelated animation", family, path)
	}
}
