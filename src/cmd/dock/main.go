package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"hachimodockplus/internal/bootstrap"
	"hachimodockplus/internal/config"
	deviceclock "hachimodockplus/internal/platform/clock"
	"hachimodockplus/internal/platform/system"
	"hachimodockplus/internal/ui/lvgl"
	"hachimodockplus/internal/ui/terminal"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("config", ".local/config.json", "configuration file")
	ui := flag.String("ui", "lvgl", "lvgl or terminal")
	preview := flag.String("preview", "", "render LVGL into a PNG instead of opening devices")
	page := flag.String("page", "catalog", "initial page: catalog, screensaver or a registered application ID")
	prepareSystem := flag.Bool("prepare-system", false, "prepare staged SMB configuration while the UI service is stopped")
	ask := flag.String("ask", "", "one text request through embedded PicoClaw")
	ttsText := flag.String("tts", "", "synthesize text to -pcm-output for service verification")
	pcmOutput := flag.String("pcm-output", "", "new raw PCM output file, mono PCM16/24kHz")
	asrFile := flag.String("asr-pcm", "", "transcribe a raw PCM16 mono/16kHz file at real-time speed")
	syncTime := flag.Bool("sync-time", false, "synchronize the device clock with SNTP (root required)")
	ntpServers := flag.String("ntp-servers", "ntp.aliyun.com:123,time.cloudflare.com:123,pool.ntp.org:123", "comma-separated SNTP servers")
	flag.Parse()
	if *prepareSystem {
		cfg, err := config.Load(*path)
		if err != nil {
			return err
		}
		if err := bootstrap.PrepareAgentExtensions(cfg); err != nil {
			return err
		}
		if err := system.PrepareConfiguration(*path); err != nil {
			return err
		}
		fmt.Println("配置共享目录已准备")
		return nil
	}
	if *syncTime {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		for _, server := range strings.Split(*ntpServers, ",") {
			t, err := deviceclock.Query(ctx, strings.TrimSpace(server))
			if err != nil {
				continue
			}
			if err = deviceclock.Set(t); err != nil {
				return err
			}
			fmt.Println("系统时间已同步:", t.Format(time.RFC3339))
			return nil
		}
		return errors.New("无法完成 NTP 对时，请检查网络或指定 -ntp-servers")
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	if err := bootstrap.PrepareAgentExtensions(cfg); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(cfg.SourcePath()), "system", "config-active")); err == nil {
		if err := system.PrepareConfiguration(cfg.SourcePath()); err != nil {
			return err
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *ask != "" || *ttsText != "" || *asrFile != "" {
		services, err := bootstrap.NewPicoClawServices(cfg)
		if err != nil {
			return err
		}
		defer services.Close()
		agent, asr, tts := services.Agent, services.ASR, services.TTS
		if *ttsText != "" {
			if *pcmOutput == "" {
				return errors.New("-tts 需要 -pcm-output")
			}
			ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
			defer cancel()
			pcm, err := tts.Synthesize(ctx, *ttsText)
			if err != nil {
				return err
			}
			defer pcm.Close()
			f, err := os.OpenFile(*pcmOutput, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			n, err := io.Copy(f, pcm)
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
			if err != nil {
				os.Remove(*pcmOutput)
				return err
			}
			fmt.Printf("PCM24K bytes=%d duration=%.2fs\n", n, float64(n)/48000)
			return nil
		}
		if *asrFile != "" {
			f, err := os.Open(*asrFile)
			if err != nil {
				return err
			}
			defer f.Close()
			ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
			defer cancel()
			text, err := asr.Transcribe(ctx, &pacedPCM{ctx: ctx, r: f, started: time.Now()}, nil)
			if err == nil {
				fmt.Println(text)
			}
			return err
		}
		if *ask != "" {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			reply, err := agent.Chat(ctx, *ask, "dock-cli-check", nil)
			if err == nil {
				fmt.Println(reply)
			}
			return err
		}
	}
	desktop, err := bootstrap.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer desktop.Close()
	if *preview == "" {
		desktop.Audio.Initialize()
		if err := desktop.Pet.Start(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	if *page != "catalog" && *page != "screensaver" {
		if err = desktop.Host.Open(*page); err != nil {
			return err
		}
	}

	switch *ui {
	case "terminal":
		return terminal.Run(ctx, desktop.Host, os.Stdin, os.Stdout, desktop.Commands)
	case "lvgl":
		return lvgl.Run(ctx, desktop.Host, cfg, desktop.Status, desktop.Preferences, desktop.Pet, *preview, *page == "screensaver", desktop.Pages...)
	default:
		return errors.New("-ui 仅支持 lvgl 或 terminal")
	}
}

type pacedPCM struct {
	ctx     context.Context
	r       io.Reader
	started time.Time
	sent    int64
}

func (p *pacedPCM) Read(b []byte) (int, error) {
	target := p.started.Add(time.Duration(p.sent) * time.Second / 32000)
	if delay := time.Until(target); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-p.ctx.Done():
			return 0, p.ctx.Err()
		case <-timer.C:
		}
	}
	n, err := p.r.Read(b[:min(len(b), 3200)])
	p.sent += int64(n)
	return n, err
}
