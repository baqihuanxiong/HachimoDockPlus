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
	"log"
	"os"
	"runtime"
	"strconv"
	"time"
	"unsafe"

	"hachimodockplus/internal/config"
	"hachimodockplus/internal/core"
	"hachimodockplus/internal/pet"
	"hachimodockplus/internal/platform/controls"
	"hachimodockplus/internal/platform/display"
	"hachimodockplus/internal/platform/status"
	"hachimodockplus/internal/preferences"
	"hachimodockplus/internal/ui/standby"
)

func withStrings(values []string, call func([]*C.char)) {
	cs := make([]*C.char, len(values))
	for i, v := range values {
		cs[i] = C.CString(v)
		defer C.free(unsafe.Pointer(cs[i]))
	}
	call(cs)
}
func Run(ctx context.Context, h *core.Host, cfg config.Config, monitor *status.Monitor, prefs *preferences.Store, companion *pet.Companion, preview string, startScreensaver bool, pages ...Page) error {
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
	var output standby.Power
	if fb != "" {
		power, powerErr := display.Open(fb)
		if powerErr != nil {
			log.Printf("显示待机不可用: %v", powerErr)
		} else {
			output = power
			defer func() {
				if err := power.Close(); err != nil {
					log.Printf("恢复显示失败: %v", err)
				}
			}()
		}
	}
	standbyControl := standby.New(output, time.Now())
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
	saverKind := ""
	var revision, appRevision uint64
	petSaver := newPetScreensaver(companion)
	defer petSaver.stop()
	updateSaverClock := func(now time.Time) {
		clock, date := "等待对时", ""
		if now.Year() >= 2024 {
			local := now.In(monitor.Location)
			clock = local.Format("15:04:05")
			date = local.Format("2006-01-02 Mon")
		}
		withStrings([]string{clock, date}, func(cs []*C.char) { C.dock_screensaver_time(cs[0], cs[1]) })
	}
	wake := func() { sleeping = false; petSaver.stop(); C.dock_screensaver_restore(); appRevision = ^uint64(0) }
	enterSaver := func(kind string) {
		petSaver.stop()
		sleeping = true
		saverKind = kind
		withStrings([]string{kind}, func(cs []*C.char) { C.dock_screensaver_show(cs[0]) })
		if kind == "pet" {
			petSaver.enter(ctx)
		}
		updateSaverClock(time.Now())
	}
	if startScreensaver {
		enterSaver(prefs.Snapshot().Screensaver)
	}
	resumeDisplay := func() {
		log.Printf("按键或触摸唤醒显示")
		C.dock_input_sleep(0)
		if sleeping && saverKind == "pet" {
			petSaver.enter(ctx)
		}
		appRevision = ^uint64(0)
	}
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
	var touchActivity uint64
	started := time.Now()
	var headerUpdated time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-hardware:
			wasOff := standbyControl.Off
			shortSW3, consumed, inputErr := standbyControl.Input(e, time.Now())
			if inputErr != nil {
				log.Printf("唤醒显示失败: %v", inputErr)
			}
			if wasOff && !standbyControl.Off {
				resumeDisplay()
			}
			if shortSW3 {
				// Preserve page-specific short actions such as cancelling a
				// pending NewAPI reset; long press always belongs to the shell.
				if !sleeping && current != nil {
					if handler, ok := current.(interface{ HardwareEvent(controls.Event) string }); ok {
						if action := handler.HardwareEvent(controls.Event{Key: controls.SW3, Pressed: true}); action != "" {
							_ = h.Dispatch(core.Event{AppID: current.ID(), Action: action})
							continue
						}
					}
				}
				if !sleeping && current == nil && h.Snapshot().Active == "" {
					enterSaver(prefs.Snapshot().Screensaver)
				} else {
					back()
				}
			}
			if consumed {
				continue
			}
			if current != nil && (!sleeping || !e.Pressed) {
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
			if sleeping && !standbyControl.Off && saverKind == "pet" {
				petSaver.update(time.Now())
			}
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
			if !sleeping && !standbyControl.Off && (revision != s.Revision || appRevision != s.Application.Revision) {
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
			if activity := uint64(C.dock_input_activity()); activity != touchActivity {
				touchActivity = activity
				standbyControl.Activity(time.Now())
			}
			var native C.dock_event
			for C.dock_poll(&native) != 0 {
				e := core.Event{AppID: C.GoString(&native.app_id[0]), Action: C.GoString(&native.action[0]), Text: C.GoString(&native.text[0])}
				if standbyControl.Off {
					if e.Action == "shell.wake" {
						if err := standbyControl.Wake(time.Now()); err != nil {
							log.Printf("唤醒显示失败: %v", err)
						} else {
							resumeDisplay()
						}
					}
					continue
				}
				if sleeping {
					switch e.Action {
					case "shell.pet.touch":
						if x, err := strconv.Atoi(e.Text); err == nil {
							petSaver.touch(x, time.Now())
						}
					case "shell.wake":
						wake()
					}
					continue
				}
				switch e.Action {
				case "shell.wake", "shell.pet.touch":
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
			now := time.Now()
			task := ""
			if companion != nil {
				v := companion.Tracker.View(now)
				if v.Connected && v.State != "idle" {
					task = fmt.Sprintf("%s/%s/%d/%s", v.Source, v.SessionID, v.TransitionRevision, v.State)
				}
			}
			transition, powerErr := standbyControl.Update(now, time.Duration(prefs.Snapshot().StandbySeconds)*time.Second, task)
			if powerErr != nil {
				log.Printf("显示待机切换失败: %v", powerErr)
			}
			switch transition {
			case standby.Entered:
				log.Printf("显示进入待机")
				petSaver.stop()
				C.dock_input_sleep(1)
			case standby.TaskWake:
				log.Printf("Agent 任务唤醒显示，进入桌宠屏保")
				C.dock_input_sleep(0)
				enterSaver("pet")
			}
			previewDelay := 300 * time.Millisecond
			if startScreensaver && prefs.Snapshot().Screensaver == "pet" {
				previewDelay = 2 * time.Second
			}
			if preview != "" && time.Since(started) > previewDelay {
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
