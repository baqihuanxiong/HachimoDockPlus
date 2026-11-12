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
	"strings"
	"unsafe"

	"hachimodockplus/internal/apps/picoclaw"
	"hachimodockplus/internal/core"
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
func (p *picoPage) Render(snapshot core.AppSnapshot) error {
	s, ok := snapshot.Data.(picoclaw.State)
	if !ok {
		return fmt.Errorf("PicoClaw 页面收到错误的状态类型")
	}
	var history strings.Builder
	for _, m := range s.Messages {
		if m.Role == "user" {
			history.WriteString("你\n")
		} else {
			history.WriteString("PicoClaw\n")
		}
		history.WriteString(m.Text)
		history.WriteString("\n\n")
	}
	voice := "开始说话"
	enabled := s.AudioReady
	if s.Phase == "recording" {
		voice = "结束并发送"
		enabled = true
	} else if s.Phase != "idle" {
		voice = "停止"
		enabled = true
	}
	audioStatus := s.AudioStatus
	if !s.AudioReady {
		audioStatus = "未接入音频设备"
	}
	values := []string{snapshot.Status, s.Partial, history.String(), voice, audioStatus}
	cs := make([]*C.char, len(values))
	for i, v := range values {
		cs[i] = C.CString(v)
		defer C.free(unsafe.Pointer(cs[i]))
	}
	flag := 0
	if enabled {
		flag = 1
	}
	busy := 0
	if s.Phase != "idle" {
		busy = 1
	}
	C.dock_picoclaw_render(cs[0], cs[1], cs[2], cs[3], C.int(flag), cs[4], C.int(busy))
	return nil
}
