//go:build lvgl && cgo

package lvgl

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hachimodockplus/internal/pet"
)

func TestPetTouchCompletesAndTaskPreempts(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg required")
	}
	file := filepath.Join(t.TempDir(), "clip.mp4")
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "color=c=red:s=64x48:r=15", "-t", "0.2", "-pix_fmt", "yuv420p", file).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %s %v", out, err)
	}
	c := &pet.Companion{Pack: pet.Pack{Name: "测试桌宠", Clips: map[string]string{
		"idle.default": file, "welcome": file, "working": file, "touch.lick": file, "touch.what": file,
	}}}
	p := newPetScreensaver(c)
	defer p.stop()
	p.enter(context.Background())
	now := p.entered.Add(5 * time.Second)
	p.update(now)
	if !strings.Contains(p.labels, "未连接") || strings.Contains(p.labels, "Desktop") {
		t.Fatal("unexpected disconnected caption", p.labels)
	}
	p.touch(320, now)
	p.update(now)
	if p.state != "touch.lick" {
		t.Fatal("first touch did not start reaction", p.state)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !p.player.Finished() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !p.player.Finished() {
		t.Fatal("reaction did not complete")
	}
	p.touch(320, now.Add(500*time.Millisecond))
	p.update(now.Add(500 * time.Millisecond))
	if p.state != "touch.lick" || p.touchSequence != 1 {
		t.Fatal("short clip/repeated tap bypassed minimum reaction time")
	}
	p.update(now.Add(1300 * time.Millisecond))
	if p.state != "idle" || p.touchFamily != "" {
		t.Fatal("reaction did not return to idle", p.state)
	}
	p.touch(320, now.Add(1400*time.Millisecond))
	if p.state != "touch.what" {
		t.Fatal("next touch did not alternate", p.state)
	}
	busy := now.Add(1500 * time.Millisecond)
	if err := c.Tracker.Apply([]byte(`{"source":"codex","status":{"state":"working","title":"Agent 任务","text":"正在处理"}}`), busy); err != nil {
		t.Fatal(err)
	}
	p.update(busy)
	p.touch(320, busy)
	if p.state != "working" || p.touchFamily != "" || !strings.Contains(p.labels, "已连接") || !strings.Contains(p.labels, "Agent 任务") {
		t.Fatal("task did not take priority over touch", p.state, p.labels)
	}
	p.update(busy.Add(10 * time.Second))
	if p.state != "idle" || !strings.Contains(p.labels, "未连接") || strings.Contains(p.labels, "Agent 任务") {
		t.Fatal("stale connection/task did not clear", p.state, p.labels)
	}
	p.stop()
	p.touch(320, busy.Add(11*time.Second))
	if p.touchFamily != "" {
		t.Fatal("late touch restarted a dismissed saver")
	}
}
