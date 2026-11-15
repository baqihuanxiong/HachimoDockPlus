// Package audio keeps ALSA process details outside application state and UI code.
package audio

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Capture interface {
	io.ReadCloser
	Stop() error // Graceful EOF: retain captured audio for ASR finalization.
}

type Device interface {
	Probe() error
	Record(context.Context) (Capture, error)
	Play(context.Context, io.Reader, int) error
}

type ALSA struct {
	CaptureDevice, PlaybackDevice string
	Volume                        *Volume
}

func (a ALSA) Probe() error {
	for _, name := range []string{"arecord", "aplay"} {
		if _, err := exec.LookPath(name); err != nil {
			return errors.New("缺少 ALSA arecord/aplay")
		}
	}
	b, err := os.ReadFile("/proc/asound/cards")
	if err != nil || strings.Contains(string(b), "no soundcards") || len(b) == 0 {
		return errors.New("未检测到声卡，请连接并配置麦克风和扬声器")
	}
	return nil
}

type recording struct {
	*os.File
	cmd  *exec.Cmd
	done chan struct{}
	once sync.Once
}

func (r *recording) Stop() error {
	r.once.Do(func() {
		_ = r.cmd.Process.Signal(os.Interrupt)
		select {
		case <-r.done:
		case <-time.After(time.Second):
			_ = r.cmd.Process.Kill()
			<-r.done
		}
	})
	return nil
}

func (r *recording) Close() error { _ = r.Stop(); return r.File.Close() }

func (a ALSA) Record(ctx context.Context) (Capture, error) {
	if err := a.Probe(); err != nil {
		return nil, err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "arecord", "-q", "-D", a.CaptureDevice, "-t", "raw", "-f", "S16_LE", "-r", "16000", "-c", "1")
	cmd.Stdout = writer
	// Keep device diagnostics out of conversation text and avoid recording them.
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		reader.Close()
		writer.Close()
		return nil, errors.New("录音进程启动失败")
	}
	writer.Close()
	r := &recording{File: reader, cmd: cmd, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(r.done) }()
	return r, nil
}

func (a ALSA) Play(ctx context.Context, pcm io.Reader, rate int) error {
	release, err := prepareOutput(ctx, a.Volume)
	if err != nil {
		return err
	}
	defer release()
	device := a.PlaybackDevice
	if a.Volume != nil {
		device = a.Volume.PlaybackDevice(device)
	}
	cmd := exec.CommandContext(ctx, "aplay", "-q", "-D", device, "-t", "raw", "-f", "S16_LE", "-r", strconv.Itoa(rate), "-c", "1")
	cmd.Stdin = outputPCM(pcm, a.Volume)
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return errors.New("音频播放失败，请检查输出设备")
	}
	return nil
}
