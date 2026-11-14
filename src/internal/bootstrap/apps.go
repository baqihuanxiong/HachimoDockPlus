// Package bootstrap is the composition root: concrete apps and their UI adapters
// are registered here. Core, shared services and generic UI do not register apps.
package bootstrap

import (
	"context"
	"hachimodockplus/internal/preferences"
	"path/filepath"

	app "hachimodockplus/internal/apps/picoclaw"
	"hachimodockplus/internal/apps/settings"
	"hachimodockplus/internal/config"
	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/battery"
	"hachimodockplus/internal/platform/connectivity"
	"hachimodockplus/internal/platform/status"
	"hachimodockplus/internal/ui/lvgl"
	"hachimodockplus/internal/ui/terminal"
)

type Desktop struct {
	Preferences *preferences.Store
	Status      *status.Monitor
	Host        *core.Host
	Pages       []lvgl.Page
	Commands    terminal.Commands
}

func New(ctx context.Context, cfg config.Config) (*Desktop, error) {
	prefs, err := preferences.Load(filepath.Join(cfg.DataDir, "preferences.json"))
	if err != nil {
		return nil, err
	}
	monitor, err := status.New(battery.New(), connectivity.New(), cfg.UI.Timezone)
	if err != nil {
		return nil, err
	}
	registry, err := core.NewRegistry(core.Registration{
		Manifest: app.Manifest,
		New: func() (core.Application, error) {
			services, err := NewPicoClawServices(cfg)
			if err != nil {
				return nil, err
			}
			return app.New(services.Agent, services.ASR, services.TTS, services.Audio, services.Close), nil
		},
	}, core.Registration{Manifest: settings.Manifest, New: func() (core.Application, error) { return settings.New(connectivity.New(), prefs), nil }})
	if err != nil {
		return nil, err
	}
	return &Desktop{
		Preferences: prefs,
		Status:      monitor,
		Host:        core.NewHost(ctx, registry),
		Pages:       []lvgl.Page{lvgl.NewPicoClawPage(), lvgl.NewSettingsPage()},
		Commands:    terminal.Commands{settings.ID: {"/refresh": "refresh", "/scan": "wifi.scan", "/bt-scan": "bt.scan"}, app.ID: {"": app.ActionSubmit, "/voice": app.ActionVoice, "/stop": app.ActionCancel, "/new": app.ActionNew}},
	}, nil
}
