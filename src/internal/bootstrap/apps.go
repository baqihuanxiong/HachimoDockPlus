// Package bootstrap is the composition root: concrete apps and their UI adapters
// are registered here. Core, shared services and generic UI do not register apps.
package bootstrap

import (
	"context"
	"path/filepath"

	"hachimodockplus/internal/apps/music"
	"hachimodockplus/internal/apps/newapi"
	app "hachimodockplus/internal/apps/picoclaw"
	"hachimodockplus/internal/apps/settings"
	"hachimodockplus/internal/config"
	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/audio"
	"hachimodockplus/internal/platform/battery"
	"hachimodockplus/internal/platform/connectivity"
	"hachimodockplus/internal/platform/netease"
	"hachimodockplus/internal/platform/status"
	"hachimodockplus/internal/preferences"
	"hachimodockplus/internal/ui/lvgl"
	"hachimodockplus/internal/ui/terminal"
)

type Desktop struct {
	Audio       *audio.Volume
	Preferences *preferences.Store
	Status      *status.Monitor
	Host        *core.Host
	Pages       []lvgl.Page
	Commands    terminal.Commands
}

func New(ctx context.Context, cfg config.Config) (*Desktop, error) {
	if err := cfg.InitializeAudio(); err != nil {
		return nil, err
	}
	prefs, err := preferences.Load(filepath.Join(cfg.DataDir, "preferences.json"))
	if err != nil {
		return nil, err
	}
	monitor, err := status.New(battery.New(), connectivity.New(), cfg.UI.Timezone)
	if err != nil {
		return nil, err
	}
	audioConfig := cfg
	volume, err := audio.NewConfiguredVolume(audio.VolumeState{Volume: cfg.Audio.Volume, Output: cfg.Audio.Output, BluetoothDevice: cfg.Audio.BluetoothDevice}, audio.OutputDevices{Speaker: cfg.Audio.PlaybackDevice, SPDIF: cfg.Audio.SPDIFDevice}, audio.HardwareMixer{Card: cfg.Audio.MixerCard}, func(s audio.VolumeState) error { return audioConfig.SaveAudio(s.Volume, s.Output, s.BluetoothDevice) })
	if err != nil {
		return nil, err
	}
	registry, err := applicationRegistry(cfg, prefs, volume)
	if err != nil {
		volume.Close()
		return nil, err
	}
	return &Desktop{
		Audio:       volume,
		Preferences: prefs,
		Status:      monitor,
		Host:        core.NewHost(ctx, registry),
		Pages:       []lvgl.Page{lvgl.NewPicoClawPage(), lvgl.NewSettingsPage(), lvgl.NewMusicPage(), lvgl.NewNewAPIPage()},
		Commands: terminal.Commands{
			newapi.ID:   {"/refresh": newapi.ActionRefresh, "/reset": newapi.ActionReset, "/confirm": newapi.ActionConfirm, "/cancel": newapi.ActionCancel},
			music.ID:    {"/play": "play.toggle", "/next": "track.next", "/previous": "track.previous", "/logout": "logout"},
			settings.ID: {"/refresh": "refresh", "/scan": "wifi.scan", "/bt-scan": "bt.scan"},
			app.ID:      {"": app.ActionSubmit, "/voice": app.ActionVoice, "/stop": app.ActionCancel, "/new": app.ActionNew},
		},
	}, nil
}

func (d *Desktop) Close() error {
	err := d.Host.Close()
	d.Audio.Close()
	return err
}

// applicationRegistry only assembles factories; validation can pass nil services.
func applicationRegistry(cfg config.Config, prefs *preferences.Store, volume *audio.Volume) (*core.Registry, error) {
	registry, err := core.NewRegistry(core.Registration{
		Manifest: app.Manifest,
		New: func() (core.Application, error) {
			services, err := NewPicoClawServices(cfg)
			if err != nil {
				return nil, err
			}
			services.Audio.Volume = volume
			return app.New(services.Agent, services.ASR, services.TTS, services.Audio, services.Close), nil
		},
	}, core.Registration{
		Manifest: settings.Manifest,
		New: func() (core.Application, error) {
			return settings.NewWithSystem(connectivity.New(), prefs, volume, systemServices(cfg)), nil
		},
	}, core.Registration{Manifest: music.Manifest, New: func() (core.Application, error) {
		client, err := netease.New(filepath.Join(cfg.AppDataDir(music.ID), "session.json"))
		if err != nil {
			return nil, err
		}
		return music.New(client, &audio.StreamPlayer{Device: cfg.Audio.PlaybackDevice, Volume: volume}, volume), nil
	}}, core.Registration{Manifest: newapi.Manifest, New: func() (core.Application, error) {
		c, err := newapi.LoadConfig(cfg)
		if err != nil {
			return nil, err
		}
		client, err := newapi.NewClient(c)
		if err != nil {
			return nil, err
		}
		return newapi.New(c, client), nil
	}})
	if err != nil {
		return nil, err
	}
	return registry.Ordered(cfg.UI.AppOrder)
}
