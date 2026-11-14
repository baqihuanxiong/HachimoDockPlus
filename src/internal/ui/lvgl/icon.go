package lvgl

import (
	"errors"
	"image/color"
	"image/png"
	"io"
	"os"
)

// iconPixels decodes local PNGs into straight-alpha BGRA for LVGL ARGB8888.
// Limit both encoded input and dimensions so application icons stay bounded.
func iconPixels(path string) ([]byte, int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(io.LimitReader(f, 4<<20))
	if err != nil {
		return nil, 0, 0, err
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 512 || cfg.Height > 512 {
		return nil, 0, 0, errors.New("应用图标尺寸必须在 1～512 像素之间")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return nil, 0, 0, err
	}
	img, err := png.Decode(io.LimitReader(f, 4<<20))
	if err != nil {
		return nil, 0, 0, err
	}
	bounds := img.Bounds()
	data := make([]byte, cfg.Width*cfg.Height*4)
	for y := 0; y < cfg.Height; y++ {
		for x := 0; x < cfg.Width; x++ {
			pixel := color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			i := (y*cfg.Width + x) * 4
			data[i] = pixel.B
			data[i+1] = pixel.G
			data[i+2] = pixel.R
			data[i+3] = pixel.A
		}
	}
	return data, cfg.Width, cfg.Height, nil
}
