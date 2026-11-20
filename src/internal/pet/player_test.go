package pet

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPlayerRealFramesSwitchAndCancel(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg required")
	}
	file := filepath.Join(t.TempDir(), "test.mp4")
	cmd := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "color=c=red:s=64x48:r=15", "-t", "0.2", "-pix_fmt", "yuv420p", file)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %s %v", out, err)
	}
	p := NewPlayer()
	defer p.Stop()
	p.Play(context.Background(), file)
	pixels := make([]byte, FrameBytes)
	var revision uint64
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		next, err := p.CopyFrame(pixels, revision)
		if err != nil {
			t.Fatal(err)
		}
		if next != revision {
			revision = next
			if revision >= 5 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	// More frames than the short source proves the decode loop actually repeats.
	if revision < 5 || pixels[2] < 200 || pixels[0] > 20 {
		t.Fatal("frames missing, incorrect color or loop failure", revision, pixels[:4])
	}
	before := time.Now()
	p.Stop()
	if time.Since(before) > time.Second {
		t.Fatal("decoder cancellation blocked")
	}
	// Touch clips end normally, retain their final frame, and can replay the
	// same source. Lifecycle clips above must continue looping instead.
	for replay := 0; replay < 2; replay++ {
		p.PlayOnce(context.Background(), file)
		deadline = time.Now().Add(3 * time.Second)
		for !p.Finished() && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		next, err := p.CopyFrame(pixels, revision)
		if !p.Finished() || err != nil || next <= revision {
			t.Fatal("one-shot clip did not finish/replay cleanly", p.Finished(), next, err)
		}
		revision = next
	}
	p.Stop()
	p.Play(context.Background(), filepath.Join(t.TempDir(), "missing.mp4"))
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, err := p.CopyFrame(pixels, revision)
		if err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("invalid input did not report failure")
}
