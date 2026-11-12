// Package bootstrap is the composition root: concrete apps and their UI adapters
// are registered here. Core, shared services and generic UI do not register apps.
package bootstrap

import (
	"context"

	app "hachimodockplus/internal/apps/picoclaw"
	"hachimodockplus/internal/config"
	"hachimodockplus/internal/core"
	"hachimodockplus/internal/ui/lvgl"
	"hachimodockplus/internal/ui/terminal"
)

type Desktop struct {
	Host     *core.Host
	Pages    []lvgl.Page
	Commands terminal.Commands
}

func New(ctx context.Context, cfg config.Config) (*Desktop, error) {
	registry, err := core.NewRegistry(core.Registration{
		Manifest: app.Manifest,
		New: func() (core.Application, error) {
			services, err := NewPicoClawServices(cfg)
			if err != nil {
				return nil, err
			}
			return app.New(services.Agent, services.ASR, services.TTS, services.Audio, services.Close), nil
		},
	})
	if err != nil {
		return nil, err
	}
	return &Desktop{
		Host:     core.NewHost(ctx, registry),
		Pages:    []lvgl.Page{lvgl.NewPicoClawPage()},
		Commands: terminal.Commands{app.ID: {"": app.ActionSubmit, "/voice": app.ActionVoice, "/stop": app.ActionCancel, "/new": app.ActionNew}},
	}, nil
}
