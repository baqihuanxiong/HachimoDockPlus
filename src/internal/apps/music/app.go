// Package music owns the foreground NetEase player and account state.
package music

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/audio"
	"hachimodockplus/internal/platform/controls"
	"hachimodockplus/internal/platform/netease"
)

func smsRemaining(deadline time.Time) int {
	if !time.Now().Before(deadline) {
		return 0
	}
	return int(time.Until(deadline).Seconds()) + 1
}

type SMSCredentials struct {
	Phone   string `json:"phone"`
	Captcha string `json:"captcha"`
}

const ID = "music"

var Manifest = core.Manifest{ID: ID, Title: "网易云音乐", Description: "我的歌单 · 封面与歌词", Icon: "\uf001", IconImage: "assets/app-icons/netease.png", IconImageFullBleed: true, Capabilities: []core.Capability{core.Network, core.Speaker}}

type Service interface {
	Account(context.Context) (netease.Profile, error)
	Cover(context.Context, string) (netease.Artwork, error)
	Lyrics(context.Context, int64) ([]netease.LyricLine, error)
	Playlists(context.Context, int64) ([]netease.Playlist, error)
	Tracks(context.Context, int64) ([]netease.Track, error)
	SongURL(context.Context, int64) (netease.Source, error)
	SendSMS(context.Context, string) error
	LoginSMS(context.Context, string, string) (netease.Profile, error)
	Logout() error
}
type Player interface {
	PlayFrom(context.Context, string, int) error
	Pause(bool) error
}
type State struct {
	Profile                               netease.Profile
	Cover                                 netease.Artwork
	Lyrics                                []netease.LyricLine
	LyricsLoading                         bool
	SMSCooldown                           int
	Playlists                             []netease.Playlist
	PlaylistID                            int64
	Tracks                                []netease.Track
	Index                                 int
	TrackPage                             int
	Playing, Paused, Loading, Busy, Trial bool
	Elapsed                               int
	Volume                                audio.VolumeState
}
type App struct {
	mu, worker, playMu      sync.Mutex
	api                     Service
	player                  Player
	volume                  *audio.Volume
	state                   State
	status                  string
	revision, epoch, playID uint64
	ctx                     context.Context
	cancel, playCancel      context.CancelFunc
	mediaCancel             context.CancelFunc
	events                  chan core.Event
	wg                      sync.WaitGroup
	smsRetryAt              time.Time
	closed                  bool
}

func New(api Service, player Player, volume *audio.Volume) *App {
	return &App{api: api, player: player, volume: volume, state: State{Index: -1}, revision: 1, status: "连接网易云音乐…"}
}
func (a *App) Start(parent context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return core.ErrClosed
	}
	if a.cancel != nil {
		return nil
	}
	a.ctx, a.cancel = context.WithCancel(parent)
	a.epoch++
	epoch, ctx := a.epoch, a.ctx
	a.events = make(chan core.Event, 8)
	events := a.events
	a.state.Busy = true
	a.revision++
	a.wg.Add(1)
	go a.run(ctx, epoch, events)
	return nil
}
func (a *App) update(ctx context.Context, epoch uint64, fn func()) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if ctx.Err() != nil || a.closed || epoch != a.epoch {
		return false
	}
	fn()
	a.revision++
	return true
}
func (a *App) run(ctx context.Context, epoch uint64, events <-chan core.Event) {
	defer a.wg.Done()
	a.worker.Lock()
	defer a.worker.Unlock()
	if ctx.Err() != nil {
		return
	}
	fail := func(err error) { a.update(ctx, epoch, func() { a.state.Busy = false; a.status = err.Error() }) }
	playlists := func() {
		a.mu.Lock()
		uid := a.state.Profile.ID
		a.mu.Unlock()
		lists, err := a.api.Playlists(ctx, uid)
		if err != nil {
			fail(err)
			return
		}
		a.update(ctx, epoch, func() {
			a.state.Playlists = lists
			a.state.Busy = false
			a.status = "选择歌单，开始听歌"
			if len(lists) == 0 {
				a.status = "这个账号还没有歌单"
			}
		})
	}
	account, err := a.api.Account(ctx)
	if err != nil {
		fail(err)
	} else if account.ID != 0 {
		a.update(ctx, epoch, func() { a.state.Profile = account })
		playlists()
	} else {
		a.update(ctx, epoch, func() {
			a.state = State{Index: -1, SMSCooldown: smsRemaining(a.smsRetryAt)}
			a.status = "请输入手机号，获取短信验证码"
		})
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			a.mu.Lock()
			if !a.smsRetryAt.IsZero() && epoch == a.epoch {
				a.state.SMSCooldown = smsRemaining(a.smsRetryAt)
				a.revision++
				if a.state.SMSCooldown == 0 {
					a.smsRetryAt = time.Time{}
				}
			}
			a.mu.Unlock()
		case e := <-events:
			switch e.Action {
			case "sms.send":
				err := a.api.SendSMS(ctx, e.Text)
				e.Text = ""
				if err != nil {
					fail(err)
					continue
				}
				a.update(ctx, epoch, func() {
					a.smsRetryAt = time.Now().Add(time.Minute)
					a.state.SMSCooldown = 60
					a.state.Busy = false
					a.status = "验证码已发送，请查看手机短信"
				})
			case "sms.login":
				var credentials SMSCredentials
				err := json.Unmarshal([]byte(e.Text), &credentials)
				e.Text = ""
				if err != nil {
					fail(errors.New("登录输入无效"))
					continue
				}
				profile, err := a.api.LoginSMS(ctx, credentials.Phone, credentials.Captcha)
				credentials = SMSCredentials{}
				if err != nil {
					fail(err)
					continue
				}
				a.update(ctx, epoch, func() {
					a.state.Profile = profile
					a.state.SMSCooldown = 0
					a.smsRetryAt = time.Time{}
					a.status = "登录成功，正在读取歌单…"
				})
				playlists()
			case "logout":
				if err := a.api.Logout(); err != nil {
					fail(err)
					continue
				}
				a.update(ctx, epoch, func() {
					a.cancelMediaLocked()
					a.state = State{Index: -1}
					a.status = "请输入手机号，获取短信验证码"
				})
			case "playlists.refresh":
				playlists()
			case "playlist.select":
				id, _ := strconv.ParseInt(e.Text, 10, 64)
				tracks, err := a.api.Tracks(ctx, id)
				if err != nil {
					fail(err)
					continue
				}
				a.update(ctx, epoch, func() {
					a.stopPlaybackLocked()
					a.cancelMediaLocked()
					a.state.Cover = netease.Artwork{}
					a.state.Lyrics = nil
					a.state.LyricsLoading = false
					a.state.Tracks = tracks
					a.state.TrackPage = 0
					a.state.PlaylistID = id
					a.state.Index = -1
					a.state.Elapsed = 0
					a.state.Busy = false
					a.status = "选择歌曲播放"
					if len(tracks) == 0 {
						a.status = "这个歌单还没有歌曲"
					}
				})
			}
		}
	}
}
func (a *App) cancelMediaLocked() {
	if a.mediaCancel != nil {
		a.mediaCancel()
		a.mediaCancel = nil
	}
}
func (a *App) loadMediaLocked(track netease.Track) {
	a.cancelMediaLocked()
	ctx, cancel := context.WithCancel(a.ctx)
	a.mediaCancel = cancel
	epoch := a.epoch
	a.state.Cover = netease.Artwork{}
	a.state.Lyrics = nil
	a.state.LyricsLoading = true
	a.wg.Add(2)
	go func() {
		defer a.wg.Done()
		cover, err := a.api.Cover(ctx, track.CoverURL)
		if err == nil {
			a.update(ctx, epoch, func() { a.state.Cover = cover })
		}
	}()
	go func() {
		defer a.wg.Done()
		lyrics, err := a.api.Lyrics(ctx, track.ID)
		a.update(ctx, epoch, func() {
			a.state.LyricsLoading = false
			if err == nil {
				a.state.Lyrics = lyrics
			}
		})
	}()
}
func (a *App) stopPlaybackLocked() {
	if a.playCancel != nil {
		a.playCancel()
		a.playCancel = nil
	}
	a.playID++
	a.state.Playing = false
	a.state.Paused = false
	a.state.Loading = false
}
func (a *App) playLocked(index int) {
	if len(a.state.Tracks) == 0 {
		return
	}
	index = (index + len(a.state.Tracks)) % len(a.state.Tracks)
	a.loadMediaLocked(a.state.Tracks[index])
	a.beginPlaybackLocked(index, 0, false)
}
func (a *App) beginPlaybackLocked(index, position int, paused bool) {
	if len(a.state.Tracks) == 0 {
		return
	}
	index = (index + len(a.state.Tracks)) % len(a.state.Tracks)
	a.stopPlaybackLocked()
	ctx, cancel := context.WithCancel(a.ctx)
	a.playCancel = cancel
	token, epoch, track := a.playID, a.epoch, a.state.Tracks[index]
	a.state.Index = index
	a.state.Loading = true
	a.state.Elapsed = position
	a.state.Paused = paused
	a.state.Trial = false
	a.status = "正在加载歌曲…"
	a.revision++
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		source, err := a.api.SongURL(ctx, track.ID)
		if err == nil {
			a.playMu.Lock()
			defer a.playMu.Unlock()
			if ctx.Err() != nil {
				return
			}
			if !a.update(ctx, epoch, func() {
				if token == a.playID {
					a.state.Loading = false
					a.state.Playing = true
					a.state.Trial = source.Trial
					a.status = "正在播放"
					if source.Trial {
						a.status = "正在播放试听片段"
					}
					if a.state.Paused {
						a.status = "已暂停"
					}
					_ = a.player.Pause(a.state.Paused)
				}
			}) {
				return
			}
			done := make(chan struct{})
			go func() {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-done:
						return
					case <-ctx.Done():
						return
					case <-ticker.C:
						a.update(ctx, epoch, func() {
							if token == a.playID && a.state.Playing && !a.state.Paused {
								a.state.Elapsed++
								if track.Duration > 0 {
									a.state.Elapsed = min(a.state.Elapsed, track.Duration)
								}
							}
						})
					}
				}
			}()
			err = a.player.PlayFrom(ctx, source.URL, position)
			close(done)
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if ctx.Err() != nil || epoch != a.epoch || token != a.playID || a.closed {
			return
		}
		a.state.Playing = false
		a.state.Loading = false
		a.state.Paused = false
		a.revision++
		if err != nil {
			a.status = err.Error()
			return
		}
		a.playLocked(index + 1)
	}()
}
func (a *App) Handle(e core.Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.cancel == nil {
		return core.ErrInactive
	}
	switch e.Action {
	case "tracks.previous":
		a.state.TrackPage = max(0, a.state.TrackPage-1)
		a.revision++
	case "tracks.next":
		a.state.TrackPage = min(max(0, (len(a.state.Tracks)-1)/20), a.state.TrackPage+1)
		a.revision++
	case "volume.set":
		n, err := strconv.Atoi(e.Text)
		if err != nil {
			return errors.New("音量无效")
		}
		a.volume.Set(n)
	case "volume.up":
		a.volume.Adjust(5)
	case "volume.down":
		a.volume.Adjust(-5)
	case "play.toggle":
		if a.state.Index < 0 || (!a.state.Playing && !a.state.Loading) {
			if len(a.state.Tracks) > 0 {
				a.playLocked(max(0, a.state.Index))
			}
			return nil
		}
		paused := !a.state.Paused
		if err := a.player.Pause(paused); err != nil {
			return err
		}
		a.state.Paused = paused
		if !a.state.Loading {
			if paused {
				a.status = "已暂停"
			} else if a.state.Trial {
				a.status = "正在播放试听片段"
			} else {
				a.status = "正在播放"
			}
		}
		a.revision++
	case "track.next":
		if len(a.state.Tracks) > 0 {
			a.playLocked(a.state.Index + 1)
		}
	case "track.previous":
		if len(a.state.Tracks) > 0 {
			index := a.state.Index - 1
			if a.state.Index < 0 {
				index = 0
			}
			a.playLocked(index)
		}
	case "play.seek":
		position, err := strconv.Atoi(e.Text)
		if err != nil || a.state.Index < 0 || a.state.Index >= len(a.state.Tracks) {
			return errors.New("播放位置无效")
		}
		track := a.state.Tracks[a.state.Index]
		if track.Duration <= 0 {
			return errors.New("这首歌暂不支持跳转")
		}
		if a.state.Trial {
			a.status = "试听片段暂不支持拖动进度"
			a.revision++
			return nil
		}
		a.beginPlaybackLocked(a.state.Index, max(0, min(position, track.Duration-1)), a.state.Paused)
	case "track.play":
		index, err := strconv.Atoi(e.Text)
		if err != nil || index < 0 || index >= len(a.state.Tracks) {
			return errors.New("歌曲无效")
		}
		a.playLocked(index)
	case "sms.send", "sms.login", "logout", "playlists.refresh", "playlist.select":
		if a.state.Busy {
			return errors.New("正在加载，请稍候")
		}
		if e.Action == "sms.send" || e.Action == "sms.login" {
			var err error
			if e.Action == "sms.send" {
				err = netease.ValidatePhone(e.Text)
				if err == nil && time.Now().Before(a.smsRetryAt) {
					err = fmt.Errorf("请等待 %d 秒后重新获取", int(time.Until(a.smsRetryAt).Seconds())+1)
				}
			} else {
				var credentials SMSCredentials
				if json.Unmarshal([]byte(e.Text), &credentials) != nil {
					err = errors.New("登录输入无效")
				} else {
					err = netease.ValidatePhone(credentials.Phone)
					if err == nil {
						err = netease.ValidateCaptcha(credentials.Captcha)
					}
				}
			}
			if err != nil {
				a.status = err.Error()
				a.revision++
				return nil
			}
		}
		if e.Action == "playlist.select" {
			id, err := strconv.ParseInt(e.Text, 10, 64)
			valid := false
			for _, p := range a.state.Playlists {
				if p.ID == id {
					valid = true
				}
			}
			if err != nil || !valid {
				return errors.New("歌单无效")
			}
		}
		if e.Action == "logout" {
			a.stopPlaybackLocked()
		}
		select {
		case a.events <- e:
			a.state.Busy = true
			a.status = "正在加载…"
			a.revision++
		default:
			return errors.New("操作过快，请稍候")
		}
	default:
		return core.ErrUnknownAction
	}
	return nil
}

// HardwareAction maps physical input to this application's actions.
func HardwareAction(key controls.Key) string {
	switch key {
	case controls.SW2:
		return "play.toggle"
	case controls.Left:
		return "track.previous"
	case controls.Right:
		return "track.next"
	// On this baseboard physical up is positive Y under catalog calibration.
	case controls.Up:
		return "volume.down"
	case controls.Down:
		return "volume.up"
	}
	return ""
}
func (a *App) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.epoch++
	a.stopPlaybackLocked()
	a.cancelMediaLocked()
	a.state.LyricsLoading = false
	a.state.Busy = false
	a.revision++
}
func (a *App) Close() error {
	a.Stop()
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
	a.wg.Wait()
	return nil
}
func (a *App) Snapshot() core.AppSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.state
	s.Playlists = append([]netease.Playlist(nil), s.Playlists...)
	s.Tracks = append([]netease.Track(nil), s.Tracks...)
	s.Lyrics = append([]netease.LyricLine(nil), s.Lyrics...)
	s.Volume = a.volume.Snapshot()
	text := a.status
	if s.Index >= 0 && s.Index < len(s.Tracks) {
		text = s.Tracks[s.Index].Name + " · " + s.Tracks[s.Index].Artist
	}
	return core.AppSnapshot{Revision: a.revision + s.Volume.Revision, Status: a.status, Text: text, Data: s}
}
