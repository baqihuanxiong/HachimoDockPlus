//go:build !lvgl || !cgo

package lvgl

import "hachimodockplus/internal/core"

type settingsPage struct{}

func NewSettingsPage() Page                        { return settingsPage{} }
func (settingsPage) ID() string                    { return "settings" }
func (settingsPage) Mount() error                  { return errUnavailable }
func (settingsPage) Unmount()                      {}
func (settingsPage) Render(core.AppSnapshot) error { return errUnavailable }
