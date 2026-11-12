package picoclaw

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"hachimodockplus/internal/platform/audio"
)

type testAgent struct {
	called  chan string
	release chan struct{}
}

func (a *testAgent) Chat(ctx context.Context, s, session string) (string, error) {
	a.called <- s
	if a.release != nil {
		<-a.release
	}
	return "回复：" + s, nil
}

type testASR struct{}

func (testASR) Transcribe(ctx context.Context, r io.Reader, update func(string)) (string, error) {
	update("临时文字")
	_, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return "确认文字", nil
}

type testTTS struct{}

func (testTTS) Ready() error { return nil }
func (testTTS) Synthesize(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("12")), nil
}

type testAudio struct {
	missing bool
	capture *testCapture
}

func (a testAudio) Probe() error {
	if a.missing {
		return errors.New("no audio")
	}
	return nil
}
func (a testAudio) Record(context.Context) (audio.Capture, error) { return a.capture, nil }
func (a testAudio) Play(ctx context.Context, r io.Reader, rate int) error {
	_, err := io.Copy(io.Discard, r)
	return err
}

type testCapture struct {
	*io.PipeReader
	w    *io.PipeWriter
	once sync.Once
}

func (c *testCapture) Stop() error  { c.once.Do(func() { c.w.Close() }); return nil }
func (c *testCapture) Close() error { c.Stop(); return c.PipeReader.Close() }
func wait(t *testing.T, fn func() bool) {
	t.Helper()
	end := time.Now().Add(2 * time.Second)
	for time.Now().Before(end) {
		if fn() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timeout waiting for state")
}

func TestVoiceSubmitsOnlyFinalAndReleasesResources(t *testing.T) {
	r, w := io.Pipe()
	capture := &testCapture{PipeReader: r, w: w}
	a := &testAgent{called: make(chan string, 2)}
	c := newTestApp(context.Background(), a, testASR{}, testTTS{}, testAudio{capture: capture})
	defer c.Close()
	c.ToggleVoice()
	wait(t, func() bool { return c.State().Partial == "临时文字" })
	select {
	case <-a.called:
		t.Fatal("partial submitted before STOP")
	default:
	}
	c.ToggleVoice()
	wait(t, func() bool { return c.State().Phase == "idle" })
	if got := <-a.called; got != "确认文字" {
		t.Fatal(got)
	}
	s := c.State()
	if len(s.Messages) != 2 || s.Messages[0].Text != "确认文字" {
		t.Fatal(s)
	}
}

func TestCancelDropsOldReplyAndNavigationClearsBusyState(t *testing.T) {
	release := make(chan struct{})
	a := &testAgent{called: make(chan string, 2), release: release}
	c := newTestApp(context.Background(), a, testASR{}, testTTS{}, testAudio{missing: true})
	if !c.Submit("旧请求") {
		t.Fatal("submit rejected")
	}
	<-a.called
	if c.Submit("不应并发") {
		t.Fatal("concurrent turn accepted")
	}
	c.Stop()
	close(release)
	wait(t, func() bool { return c.State().Phase == "idle" })
	c.Close()
	s := c.State()
	if len(s.Messages) != 1 || s.Messages[0].Role != "user" {
		t.Fatal("stale result entered UI", s)
	}
}

func TestTextWorksWithoutAudioAndSnapshotIsIndependent(t *testing.T) {
	a := &testAgent{called: make(chan string, 2)}
	c := newTestApp(context.Background(), a, testASR{}, testTTS{}, testAudio{missing: true})
	defer c.Close()
	if c.State().AudioReady {
		t.Fatal("missing audio reported ready")
	}
	c.Submit("你好")
	wait(t, func() bool { return len(c.State().Messages) == 2 && c.State().Phase == "idle" })
	s := c.State()
	s.Messages[0].Text = "修改副本"
	if c.State().Messages[0].Text != "你好" {
		t.Fatal("snapshot shared mutable state")
	}
	c.NewConversation()
	if len(c.State().Messages) != 0 {
		t.Fatal("new conversation retained UI history")
	}
}

func newTestApp(ctx context.Context, a Agent, r Recognizer, tts Synthesizer, device audio.Device) *App {
	app := New(a, r, tts, device, nil)
	if err := app.Start(ctx); err != nil {
		panic(err)
	}
	return app
}

func TestStopReleasesCaptureAndReentryRetainsConversation(t *testing.T) {
	r, w := io.Pipe()
	capture := &testCapture{PipeReader: r, w: w}
	a := &testAgent{called: make(chan string, 4)}
	c := newTestApp(context.Background(), a, testASR{}, testTTS{}, testAudio{capture: capture})
	defer c.Close()
	c.ToggleVoice()
	wait(t, func() bool { return c.State().Partial != "" })
	c.Stop()
	readDone := make(chan error, 1)
	go func() { _, err := capture.PipeReader.Read(make([]byte, 1)); readDone <- err }()
	select {
	case err := <-readDone:
		if err == nil {
			t.Fatal("capture remained readable after stop")
		}
	case <-time.After(2 * time.Second):
		capture.Close()
		t.Fatal("capture was not released after stop")
	}
	if c.Submit("inactive") {
		t.Fatal("inactive app accepted work")
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.Submit("first")
	wait(t, func() bool { return c.State().Phase == "idle" && len(c.State().Messages) == 2 })
	c.Stop()
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.Submit("second")
	wait(t, func() bool { return c.State().Phase == "idle" && len(c.State().Messages) == 4 })
}
