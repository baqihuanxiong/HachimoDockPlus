//go:build !lvgl || !cgo

package lvgl

import (
	"context"
	"errors"

	"hachimodockplus/internal/config"
	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/status"
	"hachimodockplus/internal/preferences"
)

var errUnavailable = errors.New("此二进制未包含 LVGL；使用构建脚本或选择 -ui terminal")

func Run(context.Context, *core.Host, config.Config, *status.Monitor, *preferences.Store, string, ...Page) error {
	return errUnavailable
}
