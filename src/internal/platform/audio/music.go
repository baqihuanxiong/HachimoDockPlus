package audio

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var mediaHTTP = &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, IdleConnTimeout: 30 * time.Second}}

type StreamPlayer struct {
	Device   string
	Volume   *Volume
	mu       sync.Mutex
	commands []*exec.Cmd
	paused   bool
}

func (p *StreamPlayer) Pause(paused bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	signal := syscall.SIGCONT
	if paused {
		signal = syscall.SIGSTOP
	}
	for _, cmd := range p.commands {
		if err := cmd.Process.Signal(signal); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return errors.New("无法暂停或继续播放")
		}
	}
	p.paused = paused
	return nil
}
func (p *StreamPlayer) Play(ctx context.Context, address string) error {
	return p.PlayFrom(ctx, address, 0)
}
func (p *StreamPlayer) PlayFrom(ctx context.Context, address string, position int) error {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("歌曲地址无效")
	}
	source, closeSource, err := streamProxy(ctx, address)
	if err != nil {
		return errors.New("无法建立音乐流")
	}
	defer closeSource()
	release, err := prepareOutput(ctx, p.Volume)
	if err != nil {
		return err
	}
	defer release()
	// Input seeking lets FFmpeg request the required byte range through Go.
	args := []string{"-nostdin", "-v", "error", "-rw_timeout", "15000000", "-protocol_whitelist", "http,tcp", "-ss", strconv.Itoa(max(0, position)), "-i", source, "-vn", "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1"}
	decoder := exec.CommandContext(ctx, "ffmpeg", args...)
	device := p.Device
	if strings.TrimSpace(device) == "" {
		device = "default"
	}
	if p.Volume != nil {
		device = p.Volume.PlaybackDevice(device)
	}
	output := exec.CommandContext(ctx, "aplay", "-q", "-D", device, "-t", "raw", "-f", "S16_LE", "-r", "48000", "-c", "2")
	reader, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer reader.Close()
	defer writer.Close()
	decoder.WaitDelay = 2 * time.Second
	decoder.Stdout = writer
	decoder.Stderr = io.Discard
	output.Stdin = outputPCM(reader, p.Volume)
	output.Stderr = io.Discard
	p.mu.Lock()
	if err = output.Start(); err == nil {
		err = decoder.Start()
		if err != nil {
			_ = output.Process.Kill()
			_ = output.Wait()
		}
	}
	if err != nil {
		p.mu.Unlock()
		return errors.New("无法启动音乐播放，请检查 FFmpeg / ALSA")
	}
	p.commands = []*exec.Cmd{decoder, output}
	if p.paused {
		for _, c := range p.commands {
			_ = c.Process.Signal(syscall.SIGSTOP)
		}
	}
	p.mu.Unlock()
	writer.Close()
	// A software-volume reader is consumed by os/exec in the parent process.
	decodeErr := decoder.Wait()
	if decodeErr != nil {
		_ = output.Process.Kill()
	}
	playErr := output.Wait()
	p.mu.Lock()
	p.commands = nil
	p.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if decodeErr != nil || playErr != nil {
		return errors.New("音乐播放中断，请检查网络或声卡")
	}
	return nil
}
