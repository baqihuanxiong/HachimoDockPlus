package bootstrap

import (
	"errors"
	"fmt"
	adapter "hachimodockplus/internal/agent/picoclaw"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hachimodockplus/internal/apps/newapi"
	app "hachimodockplus/internal/apps/picoclaw"
	"hachimodockplus/internal/config"
	"hachimodockplus/internal/pet"
	"hachimodockplus/internal/platform/system"
	"hachimodockplus/internal/speech"
)

func systemServices(installed config.Config) *system.Manager {
	path := installed.SourcePath()
	base := filepath.Dir(path)
	return system.New(path, func(data []byte, secretDir string) error {
		candidate, err := config.Parse(data, base)
		if err != nil {
			return err
		}
		if _, err = applicationRegistry(candidate, nil, nil); err != nil {
			return err
		}
		if _, err = time.LoadLocation(candidate.UI.Timezone); err != nil {
			return errors.New("显示时区无效")
		}
		petDirectory := system.CandidatePath(base, filepath.Dir(secretDir), candidate.UI.Pet.Directory)
		if _, e := os.Stat(petDirectory); e == nil {
			if _, e = pet.LoadPack(petDirectory); e != nil {
				return e
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
		}
		if _, ok := candidate.Apps[newapi.ID]; ok {
			c, err := newapi.LoadConfig(candidate)
			if err != nil {
				return err
			}
			providers := append([]newapi.ProviderConfig{{Service: c.Service}}, c.Providers...)
			for _, provider := range providers {
				relative, err := filepath.Rel(filepath.Join(base, "secrets"), provider.KeyFile)
				if err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
					provider.KeyFile = filepath.Join(secretDir, relative)
				}
				if err := provider.ValidateCredentials(); err != nil {
					return fmt.Errorf("NewAPI：%w", err)
				}
			}
		}
		font, err := os.Stat(system.CandidatePath(base, filepath.Dir(secretDir), candidate.UI.Font))
		if err != nil || !font.Mode().IsRegular() {
			return errors.New("中文字体文件不存在")
		}
		if candidate.UI.Framebuffer != "" {
			if _, err = os.Stat(candidate.UI.Framebuffer); err != nil {
				return errors.New("显示设备不存在")
			}
		}
		if candidate.UI.Input != "auto" {
			if _, err = os.Stat(candidate.UI.Input); err != nil {
				return errors.New("触摸设备不存在")
			}
		}
		if _, ok := candidate.Apps[app.ID]; ok {
			pc, err := app.LoadConfig(candidate)
			if err != nil {
				return err
			}
			extensions := pc.ExtensionsDir
			if extensions == filepath.Join(base, "agent") {
				extensions = filepath.Join(filepath.Dir(secretDir), "agent")
			}
			if err := adapter.ValidateExtensions(extensions, base); err != nil {
				return err
			}
			for name, service := range map[string]config.Service{"模型": pc.LLM.Service, "语音识别": pc.ASR.Service, "语音合成": pc.TTS.Service} {
				relative, err := filepath.Rel(filepath.Join(base, "secrets"), service.KeyFile)
				if err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
					service.KeyFile = filepath.Join(secretDir, relative)
				}
				if _, err = service.Key(); err != nil {
					return fmt.Errorf("%s：%w", name, err)
				}
			}
			if pc.TTS.Provider != speech.ProviderVolcengine {
				voice, err := os.Stat(system.CandidatePath(base, filepath.Dir(secretDir), pc.TTS.PromptWAV))
				if err != nil || !voice.Mode().IsRegular() || voice.Size() < 44 || voice.Size() > 4<<20 {
					return errors.New("语音合成参考音频无效")
				}
			}
		}
		return nil
	})
}
