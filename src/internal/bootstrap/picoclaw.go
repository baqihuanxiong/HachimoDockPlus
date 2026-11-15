package bootstrap

import (
	"encoding/json"
	"fmt"

	adapter "hachimodockplus/internal/agent/picoclaw"
	app "hachimodockplus/internal/apps/picoclaw"
	"hachimodockplus/internal/config"
	"hachimodockplus/internal/platform/audio"
	"hachimodockplus/internal/speech"
)

type PicoClawServices struct {
	Agent *adapter.Agent
	ASR   speech.ASR
	TTS   speech.TTS
	Audio audio.ALSA
}

// PrepareAgentExtensions runs before preparing an SMB generation, so existing
// user-written prompts and skills are migrated into the editable profile.
func PrepareAgentExtensions(root config.Config) error {
	raw, ok := root.Apps[app.ID]
	if !ok {
		return nil
	}
	// Preserve lazy app validation: an invalid service setting must not prevent
	// the shell or other apps from starting.
	var paths struct {
		DataDir       string `json:"data_dir"`
		ExtensionsDir string `json:"extensions_dir"`
	}
	if json.Unmarshal(raw, &paths) != nil {
		return nil
	}
	if paths.DataDir == "" {
		paths.DataDir = root.AppDataDir(app.ID)
	} else {
		paths.DataDir = root.Resolve(paths.DataDir)
	}
	if paths.ExtensionsDir == "" {
		paths.ExtensionsDir = root.Resolve("agent")
	} else {
		paths.ExtensionsDir = root.Resolve(paths.ExtensionsDir)
	}
	return adapter.EnsureExtensions(paths.DataDir, adapter.Options{ExtensionsDir: paths.ExtensionsDir})
}

func NewPicoClawServices(root config.Config) (*PicoClawServices, error) {
	cfg, err := app.LoadConfig(root)
	if err != nil {
		return nil, err
	}
	key, err := cfg.LLM.Key()
	if err != nil {
		return nil, fmt.Errorf("LLM: %w", err)
	}
	asrKey, err := cfg.ASR.Key()
	if err != nil {
		return nil, fmt.Errorf("ASR: %w", err)
	}
	ttsKey, err := cfg.TTS.Key()
	if err != nil {
		return nil, fmt.Errorf("TTS: %w", err)
	}
	agent, err := adapter.New(cfg.DataDir, cfg.LLM.URL, key, cfg.LLM.Model, adapter.Options{ExtensionsDir: cfg.ExtensionsDir})
	if err != nil {
		return nil, err
	}
	return &PicoClawServices{
		Agent: agent,
		ASR:   speech.ASR{URL: cfg.ASR.URL, Key: asrKey, Language: cfg.ASR.Language},
		TTS:   speech.TTS{URL: cfg.TTS.URL, Key: ttsKey, Mode: cfg.TTS.Mode, PromptWAV: cfg.TTS.PromptWAV, PromptText: cfg.TTS.PromptText, Instruction: cfg.TTS.Instruction},
		Audio: audio.ALSA{CaptureDevice: root.Audio.CaptureDevice, PlaybackDevice: root.Audio.PlaybackDevice},
	}, nil
}
func (s *PicoClawServices) Close() { s.Agent.Close() }
