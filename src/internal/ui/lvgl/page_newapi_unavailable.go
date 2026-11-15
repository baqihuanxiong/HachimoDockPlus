//go:build !lvgl || !cgo

package lvgl

import "hachimodockplus/internal/core"

type newapiPage struct{}

func NewNewAPIPage() Page                        { return newapiPage{} }
func (newapiPage) ID() string                    { return "newapi-monitor" }
func (newapiPage) Mount() error                  { return errUnavailable }
func (newapiPage) Unmount()                      {}
func (newapiPage) Render(core.AppSnapshot) error { return errUnavailable }
