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
	Chat(context.Context, string, string) (string, error)
}
type Recognizer interface {
	Transcribe(context.Context, io.Reader, func(string)) (string, error)
}
type Synthesizer interface {
	Ready() error
	Synthesize(context.Context, string) (io.ReadCloser, error)
}

type Message struct{ Role, Text string }
type State struct {
	Revision    uint64
	Phase       string
	Status      string
	Partial     string
	Messages    []Message
	AudioReady  bool
	AudioStatus string
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
	c.state = State{Phase: "idle", Status: "准备就绪", AudioReady: true, AudioStatus: "按下按钮开始说话"}
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

func (c *App) ToggleVoice() {
	c.mu.Lock()
	phase := c.state.Phase
	capture := c.capture
	c.mu.Unlock()
	if phase == "recording" {
		if capture != nil {
			go func() { _ = capture.Stop() }()
		}
		return
	}
	if phase != "idle" {
		c.Cancel()
		return
	}
	ctx, epoch, session, ok := c.begin("connecting", "连接语音识别…")
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
		c.state.Status = "正在聆听，点击结束并发送"
		c.state.Revision++
		c.mu.Unlock()
		stop := time.AfterFunc(30*time.Second, func() { _ = capture.Stop() })
		defer stop.Stop()
		text, err := c.asr.Transcribe(ctx, capture, func(s string) { c.update(epoch, func(st *State) { st.Partial = s }) })
		if err == nil && strings.TrimSpace(text) == "" {
			err = errors.New("没有听清，请再说一次")
		}
		if err == nil {
			err = c.respond(ctx, epoch, session, text)
		}
		c.finish(epoch, err)
	}()
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
	go func() { defer c.wg.Done(); c.finish(epoch, c.respond(ctx, epoch, session, text)) }()
	return true
}

func (c *App) respond(ctx context.Context, epoch uint64, session, text string) error {
	c.update(epoch, func(s *State) {
		s.Phase = "thinking"
		s.Status = "PicoClaw 正在思考…"
		s.Partial = ""
		s.Messages = append(s.Messages, Message{"user", text})
		if len(s.Messages) > 40 {
			s.Messages = s.Messages[len(s.Messages)-40:]
		}
	})
	reply, err := c.agent.Chat(ctx, text, session)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	c.update(epoch, func(s *State) {
		s.Messages = append(s.Messages, Message{"assistant", reply})
		s.Phase = "speaking"
		s.Status = "正在合成语音…"
	})
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
	ID           = "picoclaw"
	ActionSubmit = "submit"
	ActionVoice  = "voice.toggle"
	ActionCancel = "cancel"
	ActionNew    = "conversation.new"
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
