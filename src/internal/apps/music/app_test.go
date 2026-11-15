package music

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/audio"
	"hachimodockplus/internal/platform/controls"
	"hachimodockplus/internal/platform/netease"
)

type silentMixer struct{}

func (silentMixer) Apply(context.Context, int) error { return nil }

type fakeService struct {
	mu           sync.Mutex
	account      bool
	fail         bool
	blockTracks  chan struct{}
	trackStarted chan struct{}
	smsCalls     int
	smsBlocked   chan struct{}
}

func (s *fakeService) Account(context.Context) (netease.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.account {
		return netease.Profile{ID: 1, Name: "用户"}, nil
	}
	return netease.Profile{}, nil
}
func (s *fakeService) Cover(context.Context, string) (netease.Artwork, error) {
	return netease.Artwork{}, nil
}
func (s *fakeService) Lyrics(context.Context, int64) ([]netease.LyricLine, error) {
	return []netease.LyricLine{{At: 0, Text: "第一行"}, {At: 30000, Text: "第二行"}}, nil
}
func (s *fakeService) Playlists(context.Context, int64) ([]netease.Playlist, error) {
	if s.fail {
		return nil, errors.New("歌单请求失败")
	}
	return []netease.Playlist{{ID: 2, Name: "歌单", Count: 2}}, nil
}
func (s *fakeService) Tracks(context.Context, int64) ([]netease.Track, error) {
	if s.blockTracks != nil {
		close(s.trackStarted)
		<-s.blockTracks
	}
	return []netease.Track{{ID: 10, Name: "第一首", Duration: 100}, {ID: 11, Name: "第二首", Duration: 120}}, nil
}
func (s *fakeService) SongURL(context.Context, int64) (netease.Source, error) {
	return netease.Source{URL: "https://example.test/music"}, nil
}
func (s *fakeService) SendSMS(ctx context.Context, _ string) error {
	s.mu.Lock()
	s.smsCalls++
	s.mu.Unlock()
	if s.smsBlocked != nil {
		<-s.smsBlocked
	}
	return nil
}
func (s *fakeService) LoginSMS(context.Context, string, string) (netease.Profile, error) {
	s.mu.Lock()
	s.account = true
	s.mu.Unlock()
	return netease.Profile{ID: 1, Name: "用户"}, nil
}
func (s *fakeService) Logout() error { s.mu.Lock(); s.account = false; s.mu.Unlock(); return nil }

type fakePlayer struct {
	mu                sync.Mutex
	paused            bool
	active, maxActive int
	position          int
}

func (p *fakePlayer) PlayFrom(ctx context.Context, _ string, position int) error {
	p.mu.Lock()
	p.position = position
	p.mu.Unlock()
	p.mu.Lock()
	p.active++
	p.maxActive = max(p.maxActive, p.active)
	p.mu.Unlock()
	<-ctx.Done()
	p.mu.Lock()
	p.active--
	p.mu.Unlock()
	return ctx.Err()
}
func (p *fakePlayer) Pause(value bool) error {
	p.mu.Lock()
	p.paused = value
	p.mu.Unlock()
	return nil
}
func setup(t *testing.T, s *fakeService) (*App, *fakePlayer) {
	t.Helper()
	v, _ := audio.NewVolume("", silentMixer{})
	p := &fakePlayer{}
	a := New(s, p, v)
	t.Cleanup(func() { _ = a.Close(); v.Close() })
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return a, p
}
func waitState(t *testing.T, a *App, condition func(State) bool) State {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		s := a.Snapshot().Data.(State)
		if condition(s) {
			return s
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("state timeout", a.Snapshot())
	return State{}
}
func send(t *testing.T, a *App, action, text string) {
	t.Helper()
	if err := a.Handle(core.Event{Action: action, Text: text}); err != nil {
		t.Fatal(action, err)
	}
}
func TestPlaybackControlsVolumeAndReentry(t *testing.T) {
	a, p := setup(t, &fakeService{account: true})
	waitState(t, a, func(s State) bool { return !s.Busy && len(s.Playlists) == 1 })
	send(t, a, "playlist.select", "2")
	waitState(t, a, func(s State) bool { return !s.Busy && len(s.Tracks) == 2 })
	send(t, a, "play.toggle", "")
	waitState(t, a, func(s State) bool { return s.Playing })
	send(t, a, "play.toggle", "")
	if !a.Snapshot().Data.(State).Paused {
		t.Fatal("not paused")
	}
	send(t, a, "volume.up", "")
	if a.Snapshot().Data.(State).Volume.Volume != 65 {
		t.Fatal("volume not shared")
	}
	send(t, a, "track.next", "")
	waitState(t, a, func(s State) bool { return s.Playing && s.Index == 1 && !s.Paused })
	send(t, a, "track.next", "")
	waitState(t, a, func(s State) bool { return s.Playing && s.Index == 0 })
	copy := a.Snapshot().Data.(State)
	copy.Tracks[0].Name = "changed"
	if a.Snapshot().Data.(State).Tracks[0].Name == "changed" {
		t.Fatal("snapshot aliases tracks")
	}
	a.Stop()
	a.Stop()
	if a.Snapshot().Data.(State).Playing {
		t.Fatal("stop left playback state")
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitState(t, a, func(s State) bool { return !s.Busy })
	if a.Snapshot().Data.(State).Playing {
		t.Fatal("reentry autoplayed")
	}
	_ = a.Close()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active != 0 || p.maxActive > 1 {
		t.Fatal("overlapping or retained playback", p.active, p.maxActive)
	}
}
func TestLoginAndFailure(t *testing.T) {
	a, _ := setup(t, &fakeService{})
	waitState(t, a, func(s State) bool { return !s.Busy })
	send(t, a, "sms.login", `{"phone":"13800138000","captcha":"123456"}`)
	waitState(t, a, func(s State) bool { return s.Profile.ID == 1 && !s.Busy })
	send(t, a, "logout", "")
	waitState(t, a, func(s State) bool { return s.Profile.ID == 0 && !s.Busy })
	b, _ := setup(t, &fakeService{account: true, fail: true})
	waitState(t, b, func(s State) bool { return !s.Busy })
	if b.Snapshot().Status != "歌单请求失败" {
		t.Fatal(b.Snapshot())
	}
}
func TestLateCancelledPlaylistCannotMutateState(t *testing.T) {
	s := &fakeService{account: true, blockTracks: make(chan struct{}), trackStarted: make(chan struct{})}
	a, _ := setup(t, s)
	waitState(t, a, func(s State) bool { return !s.Busy })
	send(t, a, "playlist.select", "2")
	<-s.trackStarted
	a.Stop()
	close(s.blockTracks)
	_ = a.Close()
	if len(a.Snapshot().Data.(State).Tracks) != 0 {
		t.Fatal("late result accepted")
	}
}
func TestHardwareMapping(t *testing.T) {
	for key, want := range map[controls.Key]string{controls.SW2: "play.toggle", controls.Left: "track.previous", controls.Right: "track.next", controls.Up: "volume.up", controls.Down: "volume.down", controls.SW3: "", controls.JoystickPress: ""} {
		if HardwareAction(key) != want {
			t.Fatal(key)
		}
	}
}

func TestSMSLoginAndCooldown(t *testing.T) {
	service := &fakeService{}
	a, _ := setup(t, service)
	waitState(t, a, func(s State) bool { return !s.Busy })
	waitState(t, a, func(s State) bool { return !s.Busy })
	send(t, a, "sms.send", "123")
	service.mu.Lock()
	calls := service.smsCalls
	service.mu.Unlock()
	if calls != 0 {
		t.Fatal("invalid phone sent SMS")
	}
	send(t, a, "sms.send", "13800138000")
	waitState(t, a, func(s State) bool { return !s.Busy && s.SMSCooldown > 0 })
	send(t, a, "sms.send", "13800138000")
	service.mu.Lock()
	calls = service.smsCalls
	service.mu.Unlock()
	if calls != 1 {
		t.Fatal("cooldown bypassed", calls)
	}
	a.Stop()
	_ = a.Start(context.Background())
	waitState(t, a, func(s State) bool { return !s.Busy })
	send(t, a, "sms.send", "13800138000")
	service.mu.Lock()
	calls = service.smsCalls
	service.mu.Unlock()
	if calls != 1 {
		t.Fatal("reentry bypassed cooldown")
	}
	send(t, a, "sms.login", `{"phone":"13800138000","captcha":"123456"}`)
	waitState(t, a, func(s State) bool { return s.Profile.ID == 1 && !s.Busy })
	if strings.Contains(a.Snapshot().Text, "13800138000") || strings.Contains(a.Snapshot().Text, "123456") {
		t.Fatal("credentials in snapshot")
	}
}
func TestLateCancelledSMSCannotAlterNewPage(t *testing.T) {
	service := &fakeService{smsBlocked: make(chan struct{})}
	a, _ := setup(t, service)
	waitState(t, a, func(s State) bool { return !s.Busy })
	waitState(t, a, func(s State) bool { return !s.Busy })
	send(t, a, "sms.send", "13800138000")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		service.mu.Lock()
		started := service.smsCalls > 0
		service.mu.Unlock()
		if started {
			break
		}
		time.Sleep(time.Millisecond)
	}
	a.Stop()
	close(service.smsBlocked)
	_ = a.Close()
	if a.Snapshot().Data.(State).SMSCooldown != 0 {
		t.Fatal("late SMS updated stopped page")
	}
}

func TestSeekPreservesPauseAndLyrics(t *testing.T) {
	a, p := setup(t, &fakeService{account: true})
	waitState(t, a, func(s State) bool { return !s.Busy })
	send(t, a, "playlist.select", "2")
	waitState(t, a, func(s State) bool { return !s.Busy && len(s.Tracks) > 0 })
	send(t, a, "track.play", "0")
	waitState(t, a, func(s State) bool { return s.Playing && !s.LyricsLoading })
	send(t, a, "play.toggle", "")
	send(t, a, "play.seek", "45")
	waitState(t, a, func(s State) bool { return s.Playing && s.Paused && s.Elapsed == 45 && len(s.Lyrics) == 2 })
	deadline := time.Now().Add(time.Second)
	found := false
	for time.Now().Before(deadline) {
		p.mu.Lock()
		found = p.position == 45
		p.mu.Unlock()
		if found {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !found {
		t.Fatal("seek did not reach audio player")
	}
	copy := a.Snapshot().Data.(State)
	copy.Lyrics[0].Text = "changed"
	if a.Snapshot().Data.(State).Lyrics[0].Text == "changed" {
		t.Fatal("lyrics snapshot aliases state")
	}
	if err := a.Handle(core.Event{Action: "login.refresh"}); err != core.ErrUnknownAction {
		t.Fatal("removed QR action accepted", err)
	}
}
