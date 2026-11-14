package netease

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Artwork struct {
	Pixels        string
	Width, Height int
} // Immutable BGRA pixels; safe to share in snapshots.
type LyricLine struct {
	At   int
	Text string
} // Timestamp in milliseconds.
var lyricStamp = regexp.MustCompile(`\[(\d{1,3}):(\d{2})(?:[.:](\d{1,3}))?\]`)
var lyricOffset = regexp.MustCompile(`(?mi)\[offset:([+-]?\d+)\]`)

func ParseLyrics(raw string) []LyricLine {
	if len(raw) > 128<<10 {
		return nil
	}
	offset := 0
	if m := lyricOffset.FindStringSubmatch(raw); len(m) > 1 {
		offset, _ = strconv.Atoi(m[1])
	}
	lines := []LyricLine{}
	for _, line := range strings.Split(raw, "\n") {
		matches := lyricStamp.FindAllStringSubmatchIndex(line, -1)
		if len(matches) == 0 {
			continue
		}
		body := strings.TrimSpace(line[matches[len(matches)-1][1]:])
		if body == "" {
			continue
		}
		for _, m := range matches {
			minutes, _ := strconv.Atoi(line[m[2]:m[3]])
			seconds, _ := strconv.Atoi(line[m[4]:m[5]])
			if seconds >= 60 {
				continue
			}
			fraction := 0
			if m[6] >= 0 {
				part := line[m[6]:m[7]]
				fraction, _ = strconv.Atoi(part + strings.Repeat("0", 3-len(part)))
			}
			lines = append(lines, LyricLine{max(0, (minutes*60+seconds)*1000+fraction+offset), body})
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].At < lines[j].At })
	return lines
}
func (c *Client) Lyrics(ctx context.Context, id int64) ([]LyricLine, error) {
	var result struct {
		LRC     struct{ Lyric string }
		NoLyric bool `json:"nolyric"`
	}
	_, err := c.request(ctx, "/api/song/lyric", map[string]any{"id": id, "lv": -1, "kv": -1, "tv": -1, "rv": -1, "_nmclfl": 1}, &result)
	if err != nil {
		return nil, err
	}
	lines := ParseLyrics(result.LRC.Lyric)
	if result.NoLyric {
		return []LyricLine{{At: 0, Text: "纯音乐，请欣赏"}}, nil
	}
	// NetEase also represents instrumental tracks as a single far-future marker.
	if len(lines) == 1 && lines[0].At >= 60*60*1000 && strings.Contains(lines[0].Text, "纯音乐") {
		lines[0].At = 0
	}
	return lines, nil
}
func (c *Client) Cover(ctx context.Context, address string) (Artwork, error) {
	if address == "" {
		return Artwork{}, nil
	}
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !strings.HasSuffix(strings.ToLower(u.Hostname()), ".music.126.net") {
		return Artwork{}, errors.New("封面地址无效")
	}
	u.Scheme = "https"
	q := u.Query()
	q.Set("param", "256y256")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Artwork{}, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return Artwork{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Artwork{}, errors.New("封面暂不可用")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return Artwork{}, errors.New("封面读取失败")
	}
	return DecodeCover(data)
}
func DecodeCover(data []byte) (Artwork, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2048 || cfg.Height > 2048 {
		return Artwork{}, errors.New("封面格式无效")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Artwork{}, err
	}
	const size = 192
	pixels := make([]byte, size*size*4)
	bounds := img.Bounds()
	side := min(bounds.Dx(), bounds.Dy())
	left, top := bounds.Min.X+(bounds.Dx()-side)/2, bounds.Min.Y+(bounds.Dy()-side)/2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			p := color.NRGBAModel.Convert(img.At(left+x*side/size, top+y*side/size)).(color.NRGBA)
			i := (y*size + x) * 4
			pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = p.B, p.G, p.R, p.A
		}
	}
	return Artwork{string(pixels), size, size}, nil
}
