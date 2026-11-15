// Package picoclaw implements the voice-chat application, independently of its UI.
package picoclaw

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/audio"
)

type Agent interface {
	// Chat sends text and returns the final reply. onChunk receives the
	// accumulated visible reply as it grows and may be nil.
	Chat(context.Context, string, string, func(string)) (string, error)
}
type progressAgent interface {
	ChatWithProgress(context.Context, string, string, func(string, string)) (string, error)
}
type Recognizer interface {
	Transcribe(context.Context, io.Reader, func(string)) (string, error)
}
type Synthesizer interface {
	Ready() error
	Synthesize(context.Context, string) (io.ReadCloser, error)
}

type Message struct {
	Role, Text  string
	Process     string
	ProcessDone bool
}
type State struct {
	Revision      uint64
	Phase         string
	Status        string
	Partial       string
	Input         string
	InputRevision uint64
	Messages      []Message
	AudioReady    bool
	AudioStatus   string
	SpeechEnabled bool
}

type App struct {
	mu            sync.Mutex
	state         State
	agent         Agent
	asr           Recognizer
	tts           Synthesizer
	audio         audio.Device
	root          context.Context
	cancel        context.CancelFunc
	capture       audio.Capture
	epoch         uint64
	session       string
	wg            sync.WaitGroup
	closed        bool
	active        bool
	closeServices func()
	closeOnce     sync.Once
}

func New(agent Agent, asr Recognizer, tts Synthesizer, device audio.Device, closeServices func()) *App {
	c := &App{agent: agent, asr: asr, tts: tts, audio: device, session: sessionID(), closeServices: closeServices}
	c.state = State{Phase: "idle", Status: "准备就绪", AudioReady: true, AudioStatus: "按住 SW1 开始说话", SpeechEnabled: true}
	if err := device.Probe(); err != nil {
		c.state.AudioReady = false
		c.state.AudioStatus = err.Error()
	}
	if err := tts.Ready(); err != nil {
		c.state.AudioStatus += "；" + err.Error()
	}
	return c
}

func sessionID() string { return "dock-" + time.Now().Format("20060102T150405.000000000") }

func (c *App) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.state
	s.Messages = append([]Message(nil), s.Messages...)
	return s
}

func (c *App) NewConversation() {
	c.Cancel()
	c.mu.Lock()
	c.session = sessionID()
	c.state.Messages = nil
	c.state.Partial = ""
	c.state.Input = ""
	c.state.InputRevision++
	c.state.Status = "新对话已就绪"
	c.state.Revision++
	c.mu.Unlock()
}

func (c *App) update(epoch uint64, fn func(*State)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || epoch != c.epoch {
		return
	}
	fn(&c.state)
	c.state.Revision++
}

func (c *App) begin(phase, status string) (context.Context, uint64, string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.active || c.root.Err() != nil || c.cancel != nil {
		return nil, 0, "", false
	}
	c.epoch++
	ctx, cancel := context.WithTimeout(c.root, 3*time.Minute)
	c.cancel = cancel
	c.state.Phase = phase
	c.state.Status = status
	c.state.Partial = ""
	c.state.Revision++
	c.wg.Add(1)
	return ctx, c.epoch, c.session, true
}

func (c *App) finish(epoch uint64, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if epoch != c.epoch || c.closed {
		return
	}
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.capture = nil
	c.state.Phase = "idle"
	c.state.Partial = ""
	if err != nil {
		if errors.Is(err, context.Canceled) {
			c.state.Status = "已停止"
		} else if errors.Is(err, context.DeadlineExceeded) {
			c.state.Status = "请求超时，请重试"
		} else {
			c.state.Status = err.Error()
		}
	} else {
		c.state.Status = "准备就绪"
	}
	c.state.Revision++
}

// StartVoice begins a recording turn from idle. StopVoice ends the recording
// and places the confirmed transcript into the input box without sending it.
// ToggleVoice keeps the legacy toggle behavior for the terminal UI.
func (c *App) ToggleVoice() {
	c.mu.Lock()
	phase := c.state.Phase
	c.mu.Unlock()
	if phase == "recording" || phase == "connecting" {
		c.StopVoice()
		return
	}
	if phase != "idle" {
		c.Cancel()
		return
	}
	c.StartVoice()
}

func (c *App) StartVoice() {
	c.mu.Lock()
	phase := c.state.Phase
	c.mu.Unlock()
	if phase != "idle" {
		if phase != "recording" && phase != "connecting" {
			c.Cancel()
		}
		return
	}
	ctx, epoch, _, ok := c.begin("connecting", "连接语音识别…")
	if !ok {
		return
	}
	go func() {
		defer c.wg.Done()
		capture, err := c.audio.Record(ctx)
		if err != nil {
			c.finish(epoch, err)
			return
		}
		defer capture.Close()
		c.mu.Lock()
		if c.closed || epoch != c.epoch {
			c.mu.Unlock()
			return
		}
		c.capture = capture
		c.state.Phase = "recording"
		c.state.Status = "正在聆听，松开 SW1 结束"
		c.state.Revision++
		c.mu.Unlock()
		stop := time.AfterFunc(30*time.Second, func() { _ = capture.Stop() })
		defer stop.Stop()
		text, err := c.asr.Transcribe(ctx, capture, func(s string) { c.update(epoch, func(st *State) { st.Partial = s }) })
		if err == nil && strings.TrimSpace(text) == "" {
			err = errors.New("没有听清，请再说一次")
		}
		if err == nil {
			c.update(epoch, func(st *State) {
				st.Input = text
				st.InputRevision++
			})
		}
		c.finish(epoch, err)
		if err == nil {
			c.update(epoch, func(st *State) { st.Status = "识别完成，点击发送或按 SW2" })
		}
	}()
}

// StopVoice stops an active recording so the transcript lands in the input
// box, or cancels an in-flight reply when nothing is being recorded.
func (c *App) StopVoice() {
	c.mu.Lock()
	phase := c.state.Phase
	capture := c.capture
	c.mu.Unlock()
	if phase == "recording" || phase == "connecting" {
		if capture != nil {
			go func() { _ = capture.Stop() }()
		}
		return
	}
	if phase != "idle" {
		c.Cancel()
	}
}

func (c *App) Submit(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) > 4096 {
		return false
	}
	ctx, epoch, session, ok := c.begin("thinking", "PicoClaw 正在思考…")
	if !ok {
		return false
	}
	c.update(epoch, func(s *State) { s.Input = ""; s.InputRevision++ })
	go func() {
		defer c.wg.Done()
		err := c.respond(ctx, epoch, session, text)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		c.finish(epoch, err)
	}()
	return true
}

func (c *App) respond(ctx context.Context, epoch uint64, session, text string) error {
	c.update(epoch, func(s *State) {
		s.Phase = "thinking"
		s.Status = "PicoClaw 正在思考…"
		s.Partial = ""
		s.Messages = append(s.Messages, Message{Role: "user", Text: text})
		if len(s.Messages) > 40 {
			s.Messages = s.Messages[len(s.Messages)-40:]
		}
		// Reserve the assistant slot so the streamed reply renders in place.
		s.Messages = append(s.Messages, Message{Role: "assistant"})
	})
	onUpdate := func(chunk, process string) {
		c.update(epoch, func(s *State) {
			if n := len(s.Messages); n > 0 && s.Messages[n-1].Role == "assistant" {
				s.Messages[n-1].Text = chunk
				s.Messages[n-1].Process = process
				if chunk == "" && process != "" {
					s.Phase = "thinking"
					s.Status = "PicoClaw 正在处理…"
				} else {
					s.Phase = "streaming"
					s.Status = "PicoClaw 正在回复…"
				}
			}
		})
	}
	var reply string
	var err error
	if agent, ok := c.agent.(progressAgent); ok {
		reply, err = agent.ChatWithProgress(ctx, text, session, onUpdate)
	} else {
		reply, err = c.agent.Chat(ctx, text, session, func(chunk string) { onUpdate(chunk, "") })
	}
	if err != nil {
		c.update(epoch, func(s *State) {
			if n := len(s.Messages); n > 0 && s.Messages[n-1].Role == "assistant" {
				s.Messages[n-1].ProcessDone = true
				if s.Messages[n-1].Text == "" && s.Messages[n-1].Process == "" {
					s.Messages = s.Messages[:n-1]
				}
			}
		})
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	speak := false
	c.update(epoch, func(s *State) {
		if n := len(s.Messages); n > 0 && s.Messages[n-1].Role == "assistant" {
			s.Messages[n-1].Text = reply
			s.Messages[n-1].ProcessDone = true
		}
		speak = s.SpeechEnabled
		if speak {
			s.Phase = "speaking"
			s.Status = "正在合成语音…"
		}
	})
	if !speak {
		return nil
	}
	// Text conversations remain useful when no sound card/reference voice exists.
	if c.audio.Probe() != nil {
		c.update(epoch, func(s *State) { s.AudioReady = false })
		return errors.New("回复已显示；未检测到音频设备，暂无法播放")
	}
	pcm, err := c.tts.Synthesize(ctx, reply)
	if err != nil {
		return err
	}
	defer pcm.Close()
	c.update(epoch, func(s *State) { s.Status = "正在播放回复…" })
	return c.audio.Play(ctx, pcm, 24000)
}

// Disabling speech also interrupts synthesis/playback. A reply still being
// generated continues streaming and is kept as text.
func (c *App) SetSpeechEnabled(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.state.SpeechEnabled = enabled
	c.state.Revision++
	if !enabled && c.state.Phase == "speaking" && c.cancel != nil {
		c.cancel()
	}
}

// Speech starts only after the full reply is stored. Canceling this phase
// releases audio resources without removing the reply or disabling future speech.
func (c *App) StopSpeech() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Phase == "speaking" && c.cancel != nil {
		c.cancel()
	}
}

func (c *App) Cancel() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.epoch++
	cancel, capture := c.cancel, c.capture
	c.cancel = nil
	c.capture = nil
	c.state.Phase = "idle"
	c.state.Partial = ""
	c.state.Status = "已停止"
	if n := len(c.state.Messages); n > 0 && c.state.Messages[n-1].Role == "assistant" {
		c.state.Messages[n-1].ProcessDone = true
		if c.state.Messages[n-1].Text == "" && c.state.Messages[n-1].Process == "" {
			c.state.Messages = c.state.Messages[:n-1]
		}
	}
	c.state.Revision++
	if capture != nil {
		c.wg.Add(1)
	}
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if capture != nil {
		go func() { defer c.wg.Done(); _ = capture.Close() }()
	}
}

const (
	ID               = "picoclaw"
	ActionSubmit     = "submit"
	ActionVoice      = "voice.toggle"
	ActionVoiceOn    = "voice.start"
	ActionVoiceOff   = "voice.stop"
	ActionInputClear = "input.clear"
	ActionCancel     = "cancel"
	ActionNew        = "conversation.new"
	ActionSpeechSet  = "speech.set"
	ActionSpeechStop = "speech.stop"
)

var Manifest = core.Manifest{ID: ID, Title: "PicoClaw", Description: "和你的智能体聊一聊", Icon: "P", IconImage: "assets/app-icons/picoclaw.png", Capabilities: []core.Capability{core.Network, core.Microphone, core.Speaker}}

func (c *App) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return core.ErrClosed
	}
	c.root = ctx
	c.active = true
	c.state.Revision++
	return nil
}

func (c *App) Stop() {
	c.Cancel()
	c.mu.Lock()
	c.active = false
	c.mu.Unlock()
}

func (c *App) Handle(e core.Event) error {
	c.mu.Lock()
	active := c.active && !c.closed && c.root.Err() == nil
	c.mu.Unlock()
	if !active {
		return core.ErrInactive
	}
	switch e.Action {
	case ActionSubmit:
		if strings.TrimSpace(e.Text) == "" || len([]rune(e.Text)) > 4096 {
			return errors.New("消息不能为空且不能超过 4096 字")
		}
		if !c.Submit(e.Text) {
			return errors.New("正在处理请求，请先停止")
		}
	case ActionVoice:
		c.ToggleVoice()
	case ActionVoiceOn:
		c.StartVoice()
	case ActionVoiceOff:
		c.StopVoice()
	case ActionSpeechSet:
		if e.Text != "true" && e.Text != "false" {
			return errors.New("无效的语音播报开关值")
		}
		c.SetSpeechEnabled(e.Text == "true")
	case ActionSpeechStop:
		c.StopSpeech()
	case ActionInputClear:
		c.mu.Lock()
		c.state.Input = ""
		c.state.InputRevision++
		c.state.Revision++
		c.mu.Unlock()
	case ActionCancel:
		c.Cancel()
	case ActionNew:
		c.NewConversation()
	default:
		return core.ErrUnknownAction
	}
	return nil
}

func (c *App) Snapshot() core.AppSnapshot {
	s := c.State()
	text := ""
	if s.Phase == "idle" && len(s.Messages) > 0 && s.Messages[len(s.Messages)-1].Role == "assistant" {
		text = s.Messages[len(s.Messages)-1].Text
	}
	return core.AppSnapshot{Revision: s.Revision, Status: s.Status, Text: text, Data: s}
}

func (c *App) Close() error {
	c.closeOnce.Do(func() {
		c.Stop()
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		c.wg.Wait()
		if c.closeServices != nil {
			c.closeServices()
		}
	})
	return nil
}
