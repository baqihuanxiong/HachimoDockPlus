package netease

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"testing"
)

func TestLyricsTimestampsAndOffset(t *testing.T) {
	lines := ParseLyrics("[ti:test]\n[offset:-250]\n[00:30.50][01:00.123]第二行\n[00:00.5]第一行\n[00:90]invalid")
	if len(lines) != 3 || lines[0].At != 250 || lines[1].At != 30250 || lines[2].At != 59873 || lines[0].Text != "第一行" {
		t.Fatal(lines)
	}
	if len(ParseLyrics("纯音乐，无歌词")) != 0 {
		t.Fatal("untimed metadata parsed as lyrics")
	}
}
func TestCoverAndLyricsRequests(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.NRGBA{R: 200, G: 20, B: 10, A: 255})
		}
	}
	var encoded bytes.Buffer
	_ = png.Encode(&encoded, img)
	c := testClient(t, "", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/weapi/song/lyric" {
			fmt.Fprint(w, `{"code":200,"lrc":{"lyric":"[00:01.00]test"}}`)
		} else {
			if r.URL.Query().Get("param") != "256y256" {
				t.Error("unbounded artwork request")
			}
			_, _ = w.Write(encoded.Bytes())
		}
	})
	art, err := c.Cover(context.Background(), "https://p1.music.126.net/cover.jpg")
	if err != nil || art.Width != 192 || len(art.Pixels) != 192*192*4 || art.Pixels[0] != 10 || art.Pixels[2] != 200 {
		t.Fatal("cover decode", err)
	}
	if _, err = c.Cover(context.Background(), "https://example.com/cover"); err == nil {
		t.Fatal("unexpected cover domain accepted")
	}
	lyrics, err := c.Lyrics(context.Background(), 1)
	if err != nil || len(lyrics) != 1 || lyrics[0].At != 1000 {
		t.Fatal(lyrics, err)
	}
	if _, err = DecodeCover([]byte("invalid")); err == nil {
		t.Fatal("invalid cover accepted")
	}
}

func TestInstrumentalMarker(t *testing.T) {
	c := testClient(t, "", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":200,"lrc":{"lyric":"[99:00.00]纯音乐，请欣赏"}}`)
	})
	lines, err := c.Lyrics(context.Background(), 1)
	if err != nil || len(lines) != 1 || lines[0].At != 0 {
		t.Fatal(lines, err)
	}
}
