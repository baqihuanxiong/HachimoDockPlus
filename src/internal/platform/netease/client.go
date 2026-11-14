package netease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"
)

type Playlist struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"trackCount"`
}
type Track struct {
	ID                  int64
	Name, Artist, Album string
	CoverURL            string
	Duration            int
}
type Profile struct {
	ID   int64  `json:"userId"`
	Name string `json:"nickname"`
}
type Source struct {
	URL   string
	Trial bool
}
type Client struct {
	http       *http.Client
	base, path string
}

var ErrLoginRequired = errors.New("登录已失效，请重新输入短信验证码")

var origin, _ = url.Parse("https://music.163.com")

func New(path string) (*Client, error) {
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	client := &Client{http: &http.Client{Jar: jar, Timeout: 20 * time.Second}, base: origin.String(), path: path}
	if b, err := os.ReadFile(path); err == nil {
		var cookies []*http.Cookie
		if json.Unmarshal(b, &cookies) != nil {
			return nil, errors.New("网易云登录凭据损坏，请删除 session.json 后重新登录")
		}
		jar.SetCookies(origin, cookies)
	} else if !errors.Is(err, os.ErrNotExist) && path != "" {
		return nil, err
	}
	return client, nil
}
func (c *Client) request(ctx context.Context, path string, data map[string]any, out any) (int, error) {
	data["csrf_token"] = ""
	for _, cookie := range c.http.Jar.Cookies(origin) {
		if cookie.Name == "__csrf" {
			data["csrf_token"] = cookie.Value
		}
	}
	form, err := encrypt(data)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+strings.Replace(path, "/api/", "/weapi/", 1), strings.NewReader(form.Encode()))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", origin.String()+"/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36")
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, errors.New("连接网易云失败，请检查网络和系统时间")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("网易云 HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return 0, errors.New("网易云响应读取失败")
	}
	var status struct{ Code int }
	if json.Unmarshal(body, &status) != nil {
		return 0, errors.New("网易云响应格式无效")
	}
	if status.Code != 200 {
		if status.Code == 8821 {
			return status.Code, ErrLoginRestricted
		}
		if status.Code == 301 || status.Code == 302 {
			return status.Code, ErrLoginRequired
		}
		return status.Code, fmt.Errorf("网易云请求失败（%d）", status.Code)
	}
	if err = json.Unmarshal(body, out); err != nil {
		return status.Code, errors.New("网易云数据格式无效")
	}
	return status.Code, nil
}
func (c *Client) Account(ctx context.Context) (Profile, error) {
	var r struct{ Profile *Profile }
	_, err := c.request(ctx, "/api/w/nuser/account/get", map[string]any{}, &r)
	if errors.Is(err, ErrLoginRequired) {
		return Profile{}, nil
	}
	if err != nil {
		return Profile{}, err
	}
	if r.Profile == nil || r.Profile.ID == 0 {
		return Profile{}, nil
	}
	return *r.Profile, nil
}
func (c *Client) save() error {
	if c.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		return errors.New("无法保存网易云登录")
	}
	f, err := os.CreateTemp(filepath.Dir(c.path), ".session-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(c.http.Jar.Cookies(origin))
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), c.path)
}
func (c *Client) Logout() error {
	if c.path != "" {
		if err := os.Remove(c.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("无法删除登录凭据")
		}
	}
	cookies := c.http.Jar.Cookies(origin)
	for _, cookie := range cookies {
		cookie.MaxAge = -1
		cookie.Path = "/"
		c.http.Jar.SetCookies(origin, []*http.Cookie{cookie})
		cookie.Domain = ".music.163.com"
		c.http.Jar.SetCookies(origin, []*http.Cookie{cookie})
	}
	return nil
}
func (c *Client) Playlists(ctx context.Context, uid int64) ([]Playlist, error) {
	result := []Playlist{}
	for offset := 0; ; offset += 100 {
		var r struct {
			Playlist []Playlist
			More     bool
		}
		_, err := c.request(ctx, "/api/user/playlist", map[string]any{"uid": uid, "limit": 100, "offset": offset, "includeVideo": false}, &r)
		if err != nil {
			return nil, err
		}
		result = append(result, r.Playlist...)
		if !r.More || len(r.Playlist) == 0 {
			return result, nil
		}
	}
}
func (c *Client) Tracks(ctx context.Context, id int64) ([]Track, error) {
	var r struct {
		Playlist struct {
			TrackIDs []struct {
				ID int64 `json:"id"`
			} `json:"trackIds"`
		}
	}
	_, err := c.request(ctx, "/api/v6/playlist/detail", map[string]any{"id": id, "n": 100000, "s": 0}, &r)
	if err != nil {
		return nil, err
	}
	result := []Track{}
	// Detail can return only a preview; resolve all track IDs in bounded batches.
	for start := 0; start < len(r.Playlist.TrackIDs); start += 200 {
		ids := r.Playlist.TrackIDs[start:min(start+200, len(r.Playlist.TrackIDs))]
		payload := make([]map[string]int64, 0, len(ids))
		for _, item := range ids {
			payload = append(payload, map[string]int64{"id": item.ID})
		}
		b, _ := json.Marshal(payload)
		var songs struct {
			Songs []struct {
				ID   int64
				Name string
				DT   int
				AR   []struct{ Name string }
				AL   struct{ Name, PicURL string }
			}
		}
		_, err = c.request(ctx, "/api/v3/song/detail", map[string]any{"c": string(b)}, &songs)
		if err != nil {
			return nil, err
		}
		byID := make(map[int64]Track)
		for _, s := range songs.Songs {
			names := []string{}
			for _, a := range s.AR {
				names = append(names, a.Name)
			}
			byID[s.ID] = Track{ID: s.ID, Name: s.Name, Artist: strings.Join(names, " / "), Album: s.AL.Name, CoverURL: s.AL.PicURL, Duration: s.DT / 1000}
		}
		for _, item := range ids {
			if t, ok := byID[item.ID]; ok {
				result = append(result, t)
			}
		}
	}
	return result, nil
}
func (c *Client) SongURL(ctx context.Context, id int64) (Source, error) {
	var r struct {
		Data []struct {
			URL           string
			FreeTrialInfo json.RawMessage
		}
	}
	_, err := c.request(ctx, "/api/song/enhance/player/url", map[string]any{"ids": "[" + strconv.FormatInt(id, 10) + "]", "br": 320000}, &r)
	if err != nil {
		return Source{}, err
	}
	if len(r.Data) == 0 || r.Data[0].URL == "" {
		return Source{}, errors.New("这首歌暂不可播放，可能需要会员或无版权")
	}
	item := r.Data[0]
	u, err := url.Parse(item.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return Source{}, errors.New("歌曲地址无效")
	}
	// NetEase's media hosts support TLS; keep account-independent streams encrypted.
	u.Scheme = "https"
	return Source{URL: u.String(), Trial: len(item.FreeTrialInfo) > 0 && string(item.FreeTrialInfo) != "null"}, nil
}
