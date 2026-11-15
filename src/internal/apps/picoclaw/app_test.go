package picoclaw

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/audio"
)

type testAgent struct {
	called  chan string
	release chan struct{}
	chunks  []string
}

func (a *testAgent) Chat(ctx context.Context, s, session string, onChunk func(string)) (string, error) {
	a.called <- s
	if a.chunks != nil {
		text := ""
		for _, chunk := range a.chunks {
			text += chunk
			if onChunk != nil {
				onChunk(text)
			}
		}
		return text, nil
	}
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

type testProgressAgent struct {
	testAgent
	ready   chan struct{}
	release chan struct{}
}

func (a *testProgressAgent) ChatWithProgress(ctx context.Context, _ string, _ string, onUpdate func(string, string)) (string, error) {
	onUpdate("", "读取技能\n查询状态")
	close(a.ready)
	select {
	case <-a.release:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	onUpdate("最终答复", "读取技能\n查询状态")
	return "最终答复", nil
}

type progressTTS struct{ spoken chan string }

func (t progressTTS) Ready() error { return nil }
func (t progressTTS) Synthesize(_ context.Context, text string) (io.ReadCloser, error) {
	t.spoken <- text
	return io.NopCloser(strings.NewReader("12")), nil
}

func TestProgressSurvivesCompletionAndOnlyFinalAnswerIsSpoken(t *testing.T) {
	a := &testProgressAgent{ready: make(chan struct{}), release: make(chan struct{})}
	spoken := make(chan string, 2)
	c := newTestApp(context.Background(), a, testASR{}, progressTTS{spoken}, testAudio{})
	defer c.Close()
	if !c.Submit("检查状态") {
		t.Fatal("submit failed")
	}
	<-a.ready
	s := c.State()
	m := s.Messages[len(s.Messages)-1]
	if m.Process != "读取技能\n查询状态" || m.ProcessDone || m.Text != "" {
		t.Fatal("progress not separate while running", m)
	}
	close(a.release)
	wait(t, func() bool { return c.State().Phase == "idle" })
	s = c.State()
	m = s.Messages[len(s.Messages)-1]
	if m.Text != "最终答复" || m.Process != "读取技能\n查询状态" || !m.ProcessDone {
		t.Fatal("completion lost process", m)
	}
	if text := <-spoken; text != "最终答复" {
		t.Fatal("process was spoken", text)
	}
	select {
	case extra := <-spoken:
		t.Fatal("unexpected extra speech", extra)
	default:
	}
}

func TestCanceledProgressRemainsAvailable(t *testing.T) {
	a := &testProgressAgent{ready: make(chan struct{}), release: make(chan struct{})}
	c := newTestApp(context.Background(), a, testASR{}, testTTS{}, testAudio{})
	defer c.Close()
	c.Submit("检查状态")
	<-a.ready
	c.Cancel()
	s := c.State()
	m := s.Messages[len(s.Messages)-1]
	if m.Role != "assistant" || m.Process != "读取技能\n查询状态" || !m.ProcessDone {
		t.Fatal("cancel discarded the process", m)
	}
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

func TestVoiceFillsInputWithoutSendingAndReleasesResources(t *testing.T) {
	r, w := io.Pipe()
	capture := &testCapture{PipeReader: r, w: w}
	a := &testAgent{called: make(chan string, 2)}
	c := newTestApp(context.Background(), a, testASR{}, testTTS{}, testAudio{capture: capture})
	defer c.Close()
	c.StartVoice()
	wait(t, func() bool { return c.State().Partial == "临时文字" })
	select {
	case <-a.called:
		t.Fatal("agent called before STOP")
	default:
	}
	c.StopVoice()
	wait(t, func() bool { return c.State().Phase == "idle" })
	select {
	case <-a.called:
		t.Fatal("agent called without explicit submit")
	default:
	}
	s := c.State()
	if s.Input != "确认文字" || len(s.Messages) != 0 {
		t.Fatal(s)
	}
	if !c.Submit(s.Input) {
		t.Fatal("submit rejected")
	}
	wait(t, func() bool { return c.State().Phase == "idle" })
	if got := <-a.called; got != "确认文字" {
		t.Fatal(got)
	}
	if got := c.State().Input; got != "" {
		t.Fatal("input not cleared after submit", got)
	}
}

func TestStreamingReplyUpdatesAssistantMessage(t *testing.T) {
	a := &testAgent{called: make(chan string, 2), chunks: []string{"你好", "，世", "界"}}
	c := newTestApp(context.Background(), a, testASR{}, testTTS{}, testAudio{missing: true})
	defer c.Close()
	c.Submit("hi")
	wait(t, func() bool { return c.State().Phase == "idle" })
	s := c.State()
	if len(s.Messages) != 2 || s.Messages[1].Role != "assistant" || s.Messages[1].Text != "你好，世界" {
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

type gatedAgent struct {
	partial chan struct{}
	finish  chan struct{}
}

func (a gatedAgent) Chat(ctx context.Context, _, _ string, onChunk func(string)) (string, error) {
	onChunk("# 回复\n**正在生成")
	close(a.partial)
	select {
	case <-a.finish:
		return "# 回复\n**正在生成**\n- 已完成", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

type countingTTS struct{ calls int }

func (*countingTTS) Ready() error { return nil }
func (t *countingTTS) Synthesize(context.Context, string) (io.ReadCloser, error) {
	t.calls++
	return io.NopCloser(strings.NewReader("12")), nil
}

func TestDisableSpeechDuringStreamingKeepsMarkdownAndSkipsTTS(t *testing.T) {
	a := gatedAgent{partial: make(chan struct{}), finish: make(chan struct{})}
	tts := &countingTTS{}
	c := newTestApp(context.Background(), a, testASR{}, tts, testAudio{missing: true})
	defer c.Close()
	c.Submit("hi")
	<-a.partial
	s := c.State()
	if s.Phase != "streaming" || s.Messages[1].Text != "# 回复\n**正在生成" {
		t.Fatal("partial Markdown not published before completion", s)
	}
	if err := c.Handle(core.Event{Action: ActionSpeechSet, Text: "false"}); err != nil {
		t.Fatal(err)
	}
	c.StopSpeech() // Must leave the model stream running.
	close(a.finish)
	wait(t, func() bool { return c.State().Phase == "idle" })
	s = c.State()
	if s.SpeechEnabled || s.Messages[1].Text != "# 回复\n**正在生成**\n- 已完成" || s.Status != "准备就绪" || tts.calls != 0 {
		t.Fatal("muting lost text or started speech", s, tts.calls)
	}
	c.NewConversation()
	if c.State().SpeechEnabled {
		t.Fatal("new conversation reset the speech switch")
	}
}

type blockedTTS struct{ started chan struct{} }

func TestSpeechCanBeEnabledAgainForTheNextReply(t *testing.T) {
	a := &testAgent{called: make(chan string, 2)}
	tts := &countingTTS{}
	c := newTestApp(context.Background(), a, testASR{}, tts, testAudio{})
	defer c.Close()
	c.SetSpeechEnabled(false)
	c.Submit("quiet")
	wait(t, func() bool { return c.State().Phase == "idle" })
	if tts.calls != 0 {
		t.Fatal("disabled speech called TTS")
	}
	c.SetSpeechEnabled(true)
	c.Submit("speak")
	wait(t, func() bool { return c.State().Phase == "idle" })
	if tts.calls != 1 || !c.State().SpeechEnabled {
		t.Fatal("speech did not resume for the next reply")
	}
}

func (blockedTTS) Ready() error { return nil }
func (t blockedTTS) Synthesize(ctx context.Context, _ string) (io.ReadCloser, error) {
	close(t.started)
	<-ctx.Done()
	return nil, errors.New("TTS 服务连接失败")
}

type closedPCM struct {
	io.Reader
	closed chan struct{}
}

func (p closedPCM) Close() error { close(p.closed); return nil }

type playbackTTS struct{ closed chan struct{} }

func (playbackTTS) Ready() error { return nil }
func (t playbackTTS) Synthesize(context.Context, string) (io.ReadCloser, error) {
	return closedPCM{strings.NewReader("12"), t.closed}, nil
}

type blockedAudio struct {
	testAudio
	started chan struct{}
}

func (a blockedAudio) Play(ctx context.Context, _ io.Reader, _ int) error {
	a.started <- struct{}{}
	<-ctx.Done()
	return errors.New("音频播放失败，请检查输出设备")
}

func TestStopSpeechCancelsSynthesisAndPlaybackWithoutLosingReply(t *testing.T) {
	for _, phase := range []string{"synthesis", "playback"} {
		for _, action := range []string{ActionSpeechStop, ActionSpeechSet, ActionVoiceOn} {
			t.Run(phase+"/"+action, func(t *testing.T) {
				started, closed := make(chan struct{}, 1), make(chan struct{})
				var tts Synthesizer = blockedTTS{started}
				var device audio.Device = testAudio{}
				if phase == "playback" {
					tts, device = playbackTTS{closed}, blockedAudio{started: started}
				}
				a := &testAgent{called: make(chan string, 2)}
				c := newTestApp(context.Background(), a, testASR{}, tts, device)
				defer c.Close()
				c.Submit("hi")
				select {
				case <-started:
				case <-time.After(2 * time.Second):
					t.Fatal("speech did not start")
				}
				if err := c.Handle(core.Event{Action: action, Text: "false"}); err != nil {
					t.Fatal(err)
				}
				wait(t, func() bool { return c.State().Phase == "idle" })
				c.Close() // Wait for audio cleanup, including the PCM reader.
				s := c.State()
				if len(s.Messages) != 2 || s.Messages[1].Text != "回复：hi" || s.Status != "已停止" || s.SpeechEnabled != (action != ActionSpeechSet) {
					t.Fatal("stop lost the reply, changed the switch, or reported an audio failure", s)
				}
				if phase == "playback" {
					select {
					case <-closed:
					default:
						t.Fatal("PCM reader not closed")
					}
				}
			})
		}
	}
}
