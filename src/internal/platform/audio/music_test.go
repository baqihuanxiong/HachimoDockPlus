package audio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Child processes exercise real OS pause/resume, pipes and cancellation, while
// avoiding a sound card or an FFmpeg dependency on the test host.
func TestAudioProcessHelper(t *testing.T) {
	if os.Getenv("DOCK_AUDIO_HELPER") != "1" {
		return
	}
	mode := ""
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			mode = os.Args[i+1]
			break
		}
	}
	if mode == "ffmpeg" {
		for i, arg := range os.Args {
			if arg == "-i" && i+1 < len(os.Args) {
				res, err := http.Get(os.Args[i+1])
				if err == nil {
					_, _ = io.Copy(os.Stdout, res.Body)
					res.Body.Close()
				}
				break
			}
		}
	} else {
		_, _ = io.Copy(io.Discard, os.Stdin)
	}
	os.Exit(0)
}
func TestStreamPauseResumeAndCancel(t *testing.T) {
	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ffmpeg", "aplay"} {
		script := "#!/bin/sh\nexec '" + executable + "' -test.run=TestAudioProcessHelper -- " + name + " \"$@\"\n"
		if err = os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(filepath.Join(dir, "hachimodock-speaker-amp"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("DOCK_AUDIO_HELPER", "1")
	connected := make(chan struct{})
	disconnected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(connected)
		defer close(disconnected)
		w.Header().Set("Content-Type", "audio/mpeg")
		for {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(time.Millisecond):
				if _, err := w.Write(make([]byte, 4096)); err != nil {
					return
				}
				w.(http.Flusher).Flush()
			}
		}
	}))
	defer server.Close()
	v, _ := NewVolume("", &testMixer{})
	defer v.Close()
	p := &StreamPlayer{Volume: v}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.Play(ctx, server.URL) }()
	deadline := time.Now().Add(3 * time.Second)
	var pid int
	for time.Now().Before(deadline) {
		p.mu.Lock()
		if len(p.commands) == 2 {
			pid = p.commands[0].Process.Pid
		}
		p.mu.Unlock()
		if pid != 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("processes did not start")
	}
	select {
	case <-connected:
	case <-time.After(3 * time.Second):
		t.Fatal("media download did not start")
	}
	if err = p.Pause(true); err != nil {
		t.Fatal(err)
	}
	stopped := false
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
		if strings.Contains(string(b), "T (stopped)") {
			stopped = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !stopped {
		t.Fatal("decoder not paused")
	}
	if err = p.Pause(false); err != nil {
		t.Fatal(err)
	}
	if err = p.Pause(true); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("paused playback did not cancel")
	}
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("media request retained")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.commands) != 0 {
		t.Fatal("retained child processes")
	}
}
