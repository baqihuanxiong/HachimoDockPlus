package lvgl

import (
	"fmt"

	"hachimodockplus/internal/core"
)

// Page instances contain only UI state. All methods run on the LVGL OS thread.
// Unmount deletes native widgets; Mount can be called again on the same instance.
type Page interface {
	ID() string
	Mount() error
	Render(core.AppSnapshot) error
	Unmount()
}

func validatePages(manifests []core.Manifest, pages []Page) (map[string]Page, error) {
	result := make(map[string]Page, len(pages))
	for _, p := range pages {
		if p == nil || p.ID() == "" {
			return nil, fmt.Errorf("无效的页面注册")
		}
		if _, exists := result[p.ID()]; exists {
			return nil, fmt.Errorf("重复的页面: %s", p.ID())
		}
		result[p.ID()] = p
	}
	for _, m := range manifests {
		if result[m.ID] == nil {
			return nil, fmt.Errorf("应用 %s 缺少 LVGL 页面", m.ID)
		}
	}
	return result, nil
}
