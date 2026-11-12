//go:build !lvgl || !cgo

package lvgl

import (
	"hachimodockplus/internal/apps/picoclaw"
	"hachimodockplus/internal/core"
)

type unavailablePage struct{}

func NewPicoClawPage() Page                           { return unavailablePage{} }
func (unavailablePage) ID() string                    { return picoclaw.ID }
func (unavailablePage) Mount() error                  { return errUnavailable }
func (unavailablePage) Render(core.AppSnapshot) error { return errUnavailable }
func (unavailablePage) Unmount()                      {}
