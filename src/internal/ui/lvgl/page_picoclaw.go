//go:build lvgl && cgo

package lvgl

/*
#include <stdlib.h>
#include "bridge.h"
#include "page_picoclaw.h"
*/
import "C"
import (
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	"hachimodockplus/internal/apps/picoclaw"
	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/controls"
)

type picoPage struct{}

func NewPicoClawPage() Page  { return &picoPage{} }
func (*picoPage) ID() string { return picoclaw.ID }
func (*picoPage) Mount() error {
	if code := C.dock_picoclaw_mount(); code != 0 {
		return fmt.Errorf("PicoClaw 页面初始化失败: %d", int(code))
	}
	return nil
}
func (*picoPage) Unmount() { C.dock_picoclaw_unmount() }

// HardwareEvent maps the baseboard keys to chat actions: hold SW1 to talk,
// release it to finish the transcript, SW2 submits the input box.
func (*picoPage) HardwareEvent(e controls.Event) string {
	switch e.Key {
	case controls.SW1:
		if e.Pressed {
			return picoclaw.ActionVoiceOn
		}
		return picoclaw.ActionVoiceOff
	case controls.SW2:
		if e.Pressed {
			C.dock_picoclaw_submit()
		}
	}
	return ""
}

func (*picoPage) Render(snapshot core.AppSnapshot) error {
	s, ok := snapshot.Data.(picoclaw.State)
	if !ok {
		return fmt.Errorf("PicoClaw 页面收到错误的状态类型")
	}
	// Length-prefixed history the C side can split without ambiguity.
	var history strings.Builder
	write := func(role byte, text string) {
		history.WriteByte(role)
		history.WriteString(strconv.Itoa(len(text)))
		history.WriteByte(':')
		history.WriteString(text)
	}
	for _, m := range s.Messages {
		role := byte('a')
		if m.Role == "user" {
			role = 'u'
		}
		if m.Process != "" {
			processRole := byte('p')
			if m.ProcessDone {
				processRole = 'P'
			}
			write(processRole, m.Process)
		}
		if m.Text != "" || m.Process == "" {
			write(role, m.Text)
		}
	}
	recording := 0
	if s.Phase == "recording" || s.Phase == "connecting" {
		recording = 1
	}
	busy := 0
	if s.Phase != "idle" {
		busy = 1
	}
	speechEnabled, speaking := 0, 0
	if s.SpeechEnabled {
		speechEnabled = 1
	}
	if s.Phase == "speaking" {
		speaking = 1
	}
	hint := "按住 SW1 说话 · SW2 发送"
	switch {
	case !s.AudioReady:
		hint = "未接入音频设备，仅支持文字"
	case s.Phase == "recording":
		hint = "正在聆听… 松开 SW1 结束"
	case s.Phase == "connecting":
		hint = "正在连接语音识别…"
	case s.Phase != "idle":
		hint = "按 SW1 停止当前回复"
	}
	values := []string{snapshot.Status, s.Partial, hint, history.String(), s.Input}
	cs := make([]*C.char, len(values))
	for i, v := range values {
		cs[i] = C.CString(v)
		defer C.free(unsafe.Pointer(cs[i]))
	}
	C.dock_picoclaw_render(cs[0], cs[1], cs[2], cs[3], cs[4], C.longlong(s.InputRevision), C.int(recording), C.int(busy), C.int(speechEnabled), C.int(speaking))
	return nil
}

func (*picoPage) Back() bool { return C.dock_picoclaw_back() != 0 }
