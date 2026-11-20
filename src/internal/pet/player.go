package pet

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"
)

const Width, Height, FPS = 640, 480, 15
const FrameBytes = Width * Height * 4

// Player decodes outside the UI thread. Two Go buffers and one C display buffer
// bound memory independently of clip length. Only the latest frame is retained.
type Player struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	path     string
	loop     bool
	frame    []byte
	revision uint64
	err      error
	finished bool
	command  func(context.Context, string, ...string) *exec.Cmd
}

func NewPlayer() *Player { return &Player{command: exec.CommandContext} }

func (p *Player) Play(ctx context.Context, path string) {
	if p.path == path && p.loop {
		return
	}
	p.play(ctx, path, true)
}

// PlayOnce restarts even an identical clip so successive touches can replay it.
func (p *Player) PlayOnce(ctx context.Context, path string) { p.play(ctx, path, false) }

func (p *Player) Finished() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.finished
}

func (p *Player) play(ctx context.Context, path string, loop bool) {
	p.Stop()
	if path == "" {
		return
	}
	ctx, p.cancel = context.WithCancel(ctx)
	p.path, p.done = path, make(chan struct{})
	p.loop = loop
	p.mu.Lock()
	p.err = nil
	p.finished = false
	p.frame = nil
	p.mu.Unlock()
	go p.decode(ctx, path, p.done, loop)
}

func (p *Player) Stop() {
	if p.cancel != nil {
		p.cancel()
		<-p.done
	}
	p.cancel, p.done, p.path = nil, nil, ""
	p.mu.Lock()
	p.frame = nil
	p.err = nil
	p.finished = false
	p.mu.Unlock()
}

// CopyFrame copies into caller-owned storage, so a decoder can never mutate
// the bytes while the UI thread transfers them to LVGL.
func (p *Player) CopyFrame(dst []byte, after uint64) (uint64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(dst) != FrameBytes {
		return after, errors.New("视频帧缓冲尺寸错误")
	}
	if p.revision == after || p.frame == nil {
		return after, p.err
	}
	copy(dst, p.frame)
	return p.revision, p.err
}

func (p *Player) decode(ctx context.Context, path string, done chan struct{}, loop bool) {
	defer close(done)
	defer func() { p.mu.Lock(); p.finished = true; p.mu.Unlock() }()
	// File-only protocols prevent an imported movie from initiating network IO.
	// Filter before scaling; a high-frame-rate source is not fully RGB-converted.
	args := []string{"-nostdin", "-v", "error", "-threads", "2", "-filter_threads", "1", "-protocol_whitelist", "file,pipe"}
	if loop {
		args = append(args, "-stream_loop", "-1")
	}
	args = append(args, "-i", path, "-an", "-sn", "-dn", "-vf", "fps=15,scale=640:480:force_original_aspect_ratio=decrease,pad=640:480:(ow-iw)/2:(oh-ih)/2:black", "-pix_fmt", "bgra", "-f", "rawvideo", "pipe:1")
	cmd := p.command(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		if stdout != nil {
			_ = stdout.Close()
		}
		p.fail(ctx)
		return
	}
	buffer := make([]byte, FrameBytes)
	frames := 0
	next := time.Now()
	for {
		_, err = io.ReadFull(stdout, buffer)
		if err != nil {
			break
		}
		wait := time.Until(next)
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				err = ctx.Err()
			case <-timer.C:
			}
			if err != nil {
				break
			}
		}
		if ctx.Err() != nil {
			break
		}
		p.mu.Lock()
		old := p.frame
		p.frame = buffer
		p.revision++
		frames++
		p.mu.Unlock()
		if old == nil {
			buffer = make([]byte, FrameBytes)
		} else {
			buffer = old
		}
		next = next.Add(time.Second / FPS)
		if time.Since(next) > time.Second/FPS {
			next = time.Now()
		}
	}
	// The child can still be writing after a short/invalid frame. Close its pipe
	// and kill before Wait; cancellation always reaps the decoder.
	stdout.Close()
	if !loop && err == io.EOF && ctx.Err() == nil {
		if waitErr := cmd.Wait(); waitErr == nil && frames > 0 {
			return
		}
		p.fail(ctx)
		return
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()
	p.fail(ctx)
}

func (p *Player) fail(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	p.mu.Lock()
	p.err = errors.New("桌宠视频无法播放，请检查 MP4 和 FFmpeg")
	p.mu.Unlock()
}
