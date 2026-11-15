//go:build lvgl && cgo

// Package lvgl owns native UI calls on one locked operating-system thread.
package lvgl

/*
#cgo CFLAGS: -DLV_CONF_INCLUDE_SIMPLE -I${SRCDIR} -I${SRCDIR}/../../../.cache/lvgl
#cgo LDFLAGS: -lm
#include <stdlib.h>
#include "bridge.h"
*/
import "C"
import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"runtime"
	"time"
	"unsafe"

	"hachimodockplus/internal/config"
	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/controls"
	"hachimodockplus/internal/platform/status"
	"hachimodockplus/internal/preferences"
)

func withStrings(values []string, call func([]*C.char)) {
	cs := make([]*C.char, len(values))
	for i, v := range values {
		cs[i] = C.CString(v)
		defer C.free(unsafe.Pointer(cs[i]))
	}
	call(cs)
}
func Run(ctx context.Context, h *core.Host, cfg config.Config, monitor *status.Monitor, prefs *preferences.Store, preview string, pages ...Page) error {
	registered, err := validatePages(h.Snapshot().Applications, pages)
	if err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	fb := cfg.UI.Framebuffer
	if preview != "" {
		fb = ""
	}
	var code C.int
	withStrings([]string{fb, cfg.UI.Input, cfg.UI.Font}, func(cs []*C.char) { code = C.dock_init(cs[0], cs[1], cs[2]) })
	if code != 0 {
		C.dock_close()
		return fmt.Errorf("LVGL 初始化失败（%d）：检查显示、触摸权限及字体文件", int(code))
	}
	defer C.dock_close()
	var hardware <-chan controls.Event
	if preview == "" && !cfg.UI.Controls.Disabled {
		var stopControls func()
		hardware, stopControls = controls.Start(ctx, cfg.UI.Controls)
		defer stopControls()
	}
	currentAppearance := preferences.Snapshot{}
	applyTheme := func() {
		selected := prefs.Snapshot()
		if selected != currentAppearance {
			theme, _ := preferences.Find(selected.Theme)
			light := 0
			if selected.Mode == "light" {
				light = 1
			}
			C.dock_theme_appearance(C.uint(theme.Color), C.int(light))
			currentAppearance = selected
		}
	}
	applyTheme()
	stopMonitor := monitor.Start(ctx)
	defer stopMonitor()
	for _, m := range h.Snapshot().Applications {
		withStrings([]string{m.ID, m.Title, m.Description, m.Icon}, func(cs []*C.char) { C.dock_catalog_add(cs[0], cs[1], cs[2], cs[3]) })
		if m.IconImage != "" {
			if pixels, w, h, err := iconPixels(cfg.Resolve(m.IconImage)); err == nil {
				withStrings([]string{m.ID}, func(cs []*C.char) {
					C.dock_catalog_image(cs[0], (*C.uchar)(unsafe.Pointer(&pixels[0])), C.int(w), C.int(h))
					if m.IconImageFullBleed {
						C.dock_catalog_image_fill(cs[0])
					}
				})
			}
		}
	}
	updateRunning := func(s core.Snapshot) {
		for _, m := range s.Applications {
			withStrings([]string{m.ID}, func(cs []*C.char) {
				running := 0
				if s.Running[m.ID] {
					running = 1
				}
				C.dock_catalog_running(cs[0], C.int(running))
			})
		}
	}
	var current Page
	defer func() {
		if current != nil {
			C.dock_show_catalog()
			current.Unmount()
		}
	}()
	sleeping := false
	updateSaverClock := func(now time.Time) {
		clock, date := "等待对时", ""
		if now.Year() >= 2024 {
			local := now.In(monitor.Location)
			clock = local.Format("15:04:05")
			date = local.Format("2006-01-02 Mon")
		}
		withStrings([]string{clock, date}, func(cs []*C.char) { C.dock_screensaver_time(cs[0], cs[1]) })
	}
	wake := func() { sleeping = false; C.dock_show_catalog() }
	back := func() {
		if sleeping {
			wake()
			return
		}
		if handler, ok := current.(interface{ Back() bool }); ok && handler.Back() {
			return
		}
		h.Back()
	}
	tick := time.NewTicker(8 * time.Millisecond)
	defer tick.Stop()
	var revision, appRevision uint64
	started := time.Now()
	var headerUpdated time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-hardware:
			if !sleeping && current != nil {
				if handler, ok := current.(interface {
					HardwareEvent(controls.Event) string
				}); ok {
					// Pages that take over hardware input receive press and
					// release events; returning "" falls through to defaults.
					if action := handler.HardwareEvent(e); action != "" {
						_ = h.Dispatch(core.Event{AppID: current.ID(), Action: action})
						continue
					}
				} else if e.Pressed {
					if handler, ok := current.(interface{ Hardware(controls.Key) string }); ok {
						if action := handler.Hardware(e.Key); action != "" {
							_ = h.Dispatch(core.Event{AppID: current.ID(), Action: action})
							continue
						}
					}
				}
			}
			if !e.Pressed {
				continue
			}
			switch e.Key {
			case controls.SW2:
				if !sleeping && current == nil && h.Snapshot().Active == "" {
					C.dock_catalog_activate()
				}
			case controls.SW3:
				if !sleeping && current == nil && h.Snapshot().Active == "" {
					sleeping = true
					withStrings([]string{prefs.Snapshot().Screensaver}, func(cs []*C.char) { C.dock_screensaver_show(cs[0]) })
					updateSaverClock(time.Now())
				} else {
					back()
				}
			case controls.Up, controls.Down, controls.Left, controls.Right:
				if !sleeping && current == nil && h.Snapshot().Active == "" {
					dx, dy := 0, 0
					switch e.Key {
					case controls.Up:
						dy = -1
					case controls.Down:
						dy = 1
					case controls.Left:
						dx = -1
					case controls.Right:
						dx = 1
					}
					C.dock_catalog_move(C.int(dx), C.int(dy))
				}
			}
		case <-tick.C:
			s := h.Snapshot()
			applyTheme()
			if time.Since(headerUpdated) >= time.Second {
				headerUpdated = time.Now()
				if sleeping {
					updateSaverClock(headerUpdated)
				}
				updateRunning(s)
				st := monitor.Snapshot()
				clock := monitor.Clock(headerUpdated)
				level, charging := -1, 0
				if st.BatteryReady {
					level = st.Battery.ApproximateLevel()
					if st.Battery.Charging() {
						charging = 1
					}
				}
				withStrings([]string{clock}, func(cs []*C.char) {
					C.dock_catalog_header(cs[0], C.int(level), C.int(charging), C.int(st.Radios.WiFiStrength), C.int(st.Radios.BluetoothKind))
				})
			}
			if revision != s.Revision || appRevision != s.Application.Revision {
				updateRunning(s)
				if current != nil && current.ID() != s.Active {
					C.dock_show_catalog()
					current.Unmount()
					current = nil
				}
				if current == nil && s.Active != "" {
					current = registered[s.Active]
					if err = current.Mount(); err != nil {
						return err
					}
				}
				if current != nil {
					if err = current.Render(s.Application); err != nil {
						return err
					}
				}
				withStrings([]string{s.Status}, func(cs []*C.char) { C.dock_catalog_status(cs[0]) })
				revision, appRevision = s.Revision, s.Application.Revision
			}
			C.dock_tick()
			var native C.dock_event
			for C.dock_poll(&native) != 0 {
				e := core.Event{AppID: C.GoString(&native.app_id[0]), Action: C.GoString(&native.action[0]), Text: C.GoString(&native.text[0])}
				if sleeping {
					if e.Action == "shell.wake" {
						wake()
					}
					continue
				}
				switch e.Action {
				case "shell.wake":
					// Ignore a stale wake after physical navigation.
				case core.ActionOpen:
					if e.AppID == "" {
						_ = h.Open(e.Text)
					}
				case core.ActionBack:
					if e.AppID == h.Snapshot().Active {
						h.Back()
					}
				default:
					_ = h.Dispatch(e)
				}
			}
			if preview != "" && time.Since(started) > 300*time.Millisecond {
				return savePreview(preview)
			}
		}
	}
}
func savePreview(path string) error {
	ptr := C.dock_pixels()
	if ptr == nil {
		return errors.New("预览像素不可用")
	}
	buf := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), 640*480*4)
	img := image.NewRGBA(image.Rect(0, 0, 640, 480))
	for i := 0; i < len(buf); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = buf[i+2], buf[i+1], buf[i], 255
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
