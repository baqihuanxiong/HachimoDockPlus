//go:build lvgl && cgo

package lvgl

/*
#include "bridge.h"
*/
import "C"
import (
	"context"
	"strings"
	"time"
	"unsafe"

	"hachimodockplus/internal/pet"
)

// The shared screen owns this adapter, just as it owns the clock saver. It has
// no foreground application lifecycle and never changes application state.
type petScreensaver struct {
	companion           *pet.Companion
	player              *pet.Player
	ctx                 context.Context
	pixels              []byte
	revision, variation uint64
	state, labels       string
	entered, changed    time.Time
	touchFamily         string
	touchSequence       uint64
	touched             time.Time
}

func newPetScreensaver(companion *pet.Companion) *petScreensaver {
	return &petScreensaver{companion: companion, player: pet.NewPlayer()}
}
func (p *petScreensaver) enter(ctx context.Context) {
	p.ctx = ctx
	p.entered = time.Now()
	p.changed = time.Time{}
	p.state = ""
	p.labels = ""
	p.revision = 0
	p.touchFamily = ""
	p.pixels = make([]byte, pet.FrameBytes)
	p.update(time.Now())
}
func (p *petScreensaver) stop() { p.player.Stop(); p.pixels = nil }

func (p *petScreensaver) touch(x int, now time.Time) {
	if p.ctx == nil || len(p.pixels) == 0 || p.companion == nil || p.companion.Tracker.View(now).State != "idle" {
		return
	}
	if p.touchFamily != "" && (!p.player.Finished() || now.Sub(p.touched) < 1200*time.Millisecond) {
		return
	}
	family, path := p.companion.Pack.TouchClip(x, p.touchSequence)
	if path == "" {
		return
	}
	if family != "touch.left" && family != "touch.right" {
		p.touchSequence++
	}
	p.touchFamily, p.touched = family, now
	p.state, p.changed = family, now
	p.player.PlayOnce(p.ctx, path)
}

func (p *petScreensaver) update(now time.Time) {
	if p.ctx == nil || len(p.pixels) == 0 || p.companion == nil {
		return
	}
	v := p.companion.Tracker.View(now)
	state := v.State
	if state == "idle" && now.Sub(p.entered) < 4*time.Second {
		state = "welcome"
	}
	if p.touchFamily != "" {
		if v.State == "idle" && (!p.player.Finished() || now.Sub(p.touched) < 1200*time.Millisecond) {
			state = p.touchFamily
		} else {
			p.touchFamily = ""
		}
	}
	if state != p.state || now.Sub(p.changed) >= 20*time.Second && state == "idle" {
		_, path := p.companion.Pack.Clip(state, p.variation)
		p.player.Play(p.ctx, path)
		p.variation++
		p.state = state
		p.changed = now
	}
	revision, decodeError := p.player.CopyFrame(p.pixels, p.revision)
	if revision != p.revision {
		C.dock_screensaver_frame((*C.uchar)(unsafe.Pointer(&p.pixels[0])), C.uint(len(p.pixels)))
		p.revision = revision
	}
	connection := "未连接"
	if v.Connected {
		connection = "已连接"
	}
	caption := map[string]string{"idle": "休息中", "working": "工作中", "waiting_user": "等待确认", "done": "已完成", "error": "遇到问题", "welcome": "你好"}[state]
	if strings.HasPrefix(state, "touch.") {
		caption = "互动中"
	}
	top := p.companion.Pack.Name + " · " + connection + " · " + caption
	title, content := v.Title, v.Content
	if err := p.companion.MediaError; err != nil {
		title, content = "请导入桌宠素材", err.Error()
	} else if decodeError != nil {
		title, content = "视频暂时无法播放", decodeError.Error()
	}
	labels := strings.Join([]string{top, title, content}, "\x00")
	if p.labels != labels {
		withStrings([]string{top, title, content}, func(cs []*C.char) { C.dock_screensaver_pet(cs[0], cs[1], cs[2]) })
		p.labels = labels
	}
}
