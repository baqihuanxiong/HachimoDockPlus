//go:build !lvgl || !cgo

package lvgl

import "hachimodockplus/internal/core"

type musicPage struct{}

func NewMusicPage() Page                        { return musicPage{} }
func (musicPage) ID() string                    { return "music" }
func (musicPage) Mount() error                  { return errUnavailable }
func (musicPage) Unmount()                      {}
func (musicPage) Render(core.AppSnapshot) error { return errUnavailable }
