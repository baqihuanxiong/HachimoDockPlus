package lvgl

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestIconPixelsKeepStraightAlphaAndBGRAOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "icon.png")
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 250, G: 20, B: 40, A: 128})
	img.SetNRGBA(1, 0, color.NRGBA{R: 10, G: 200, B: 90, A: 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	pixels, w, h, err := iconPixels(path)
	if err != nil {
		t.Fatal(err)
	}
	if w != 2 || h != 1 || len(pixels) != 8 {
		t.Fatal(w, h, len(pixels))
	}
	want := []byte{40, 20, 250, 128, 90, 200, 10, 255}
	for i := range want {
		if pixels[i] != want[i] {
			t.Fatalf("pixel byte %d got %d want %d", i, pixels[i], want[i])
		}
	}
}
func TestIconRejectsMissingCorruptAndOversizedFiles(t *testing.T) {
	dir := t.TempDir()
	if _, _, _, err := iconPixels(filepath.Join(dir, "missing.png")); err == nil {
		t.Fatal("missing file accepted")
	}
	path := filepath.Join(dir, "bad.png")
	os.WriteFile(path, []byte("not an image"), 0600)
	if _, _, _, err := iconPixels(path); err == nil {
		t.Fatal("invalid PNG accepted")
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, image.NewNRGBA(image.Rect(0, 0, 513, 1)))
	f.Close()
	if _, _, _, err = iconPixels(path); err == nil {
		t.Fatal("oversized image accepted")
	}
}
