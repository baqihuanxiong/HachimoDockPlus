//go:build lvgl && cgo

package lvgl

/*
#include "page_music.h"
*/
import "C"
import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"hachimodockplus/internal/apps/music"
	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/controls"
	"hachimodockplus/internal/platform/netease"
)

type musicPage struct {
	previous music.State
	rendered bool
}

func NewMusicPage() Page      { return &musicPage{} }
func (*musicPage) ID() string { return music.ID }
func (p *musicPage) Mount() error {
	p.rendered = false
	if C.dock_music_mount() != 0 {
		return fmt.Errorf("音乐页面初始化失败")
	}
	return nil
}
func (*musicPage) Unmount()                         { C.dock_music_unmount() }
func (*musicPage) Back() bool                       { return C.dock_music_back() != 0 }
func (*musicPage) Hardware(key controls.Key) string { return music.HardwareAction(key) }
func (p *musicPage) Render(snapshot core.AppSnapshot) error {
	s, ok := snapshot.Data.(music.State)
	if !ok {
		return fmt.Errorf("音乐页面收到错误状态")
	}
	track := netease.Track{}
	if s.Index >= 0 && s.Index < len(s.Tracks) {
		track = s.Tracks[s.Index]
	}
	withStrings([]string{snapshot.Status, s.Profile.Name, track.Name, track.Artist, track.Album, s.Volume.Error}, func(cs []*C.char) {
		C.dock_music_render(cs[0], cs[1], cs[2], cs[3], cs[4], flag(s.Playing), flag(s.Paused), flag(s.Loading), C.int(s.Elapsed), C.int(track.Duration), C.int(s.Volume.Volume), cs[5], flag(s.Profile.ID != 0), flag(s.Busy), flag(track.Duration > 0 && !s.Trial && s.Index >= 0))
	})
	C.dock_music_login(C.int(s.SMSCooldown), flag(s.Busy))
	if !p.rendered || s.Cover.Pixels != p.previous.Cover.Pixels {
		var pixels *C.uchar
		if s.Cover.Pixels != "" {
			pixels = (*C.uchar)(unsafe.Pointer(unsafe.StringData(s.Cover.Pixels)))
		}
		C.dock_music_cover(pixels, C.int(s.Cover.Width), C.int(s.Cover.Height))
	}
	previous, current, next := "", "暂无歌词", ""
	if s.LyricsLoading {
		current = "歌词加载中…"
	} else if len(s.Lyrics) > 0 {
		index := sort.Search(len(s.Lyrics), func(i int) bool { return s.Lyrics[i].At > s.Elapsed*1000 }) - 1
		if index < 0 {
			current = "前奏"
			next = s.Lyrics[0].Text
		} else {
			current = s.Lyrics[index].Text
			if index > 0 {
				previous = s.Lyrics[index-1].Text
			}
			if index+1 < len(s.Lyrics) {
				next = s.Lyrics[index+1].Text
			}
		}
	}
	withStrings([]string{previous, current, next}, func(cs []*C.char) { C.dock_music_lyrics(cs[0], cs[1], cs[2]) })
	if !p.rendered || s.TrackPage != p.previous.TrackPage || s.PlaylistID != p.previous.PlaylistID || s.Index != p.previous.Index || !reflect.DeepEqual(s.Playlists, p.previous.Playlists) || !reflect.DeepEqual(s.Tracks, p.previous.Tracks) {
		C.dock_music_lists_clear()
		for _, list := range s.Playlists {
			withStrings([]string{strconv.FormatInt(list.ID, 10), strings.TrimSpace(list.Name)}, func(cs []*C.char) {
				C.dock_music_playlist(cs[0], cs[1], C.int(list.Count), flag(list.ID == s.PlaylistID))
			})
		}
		start := min(s.TrackPage*20, len(s.Tracks))
		for i := start; i < min(start+20, len(s.Tracks)); i++ {
			t := s.Tracks[i]
			withStrings([]string{t.Name, t.Artist}, func(cs []*C.char) { C.dock_music_track(C.int(i), cs[0], cs[1], flag(i == s.Index)) })
		}
		C.dock_music_tracks_page(C.int(s.TrackPage), C.int((len(s.Tracks)+19)/20), C.int(len(s.Tracks)))
	}
	p.previous = s
	p.rendered = true
	return nil
}
