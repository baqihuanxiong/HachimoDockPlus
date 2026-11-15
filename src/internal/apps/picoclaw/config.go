package picoclaw

import (
	"errors"

	"hachimodockplus/internal/config"
)

type Config struct {
	DataDir       string `json:"data_dir,omitempty"`
	ExtensionsDir string `json:"extensions_dir,omitempty"`
	LLM           struct {
		config.Service
		Model string `json:"model"`
	} `json:"llm"`
	ASR struct {
		config.Service
		Language string `json:"language"`
	} `json:"asr"`
	TTS struct {
		config.Service
		Mode        string `json:"mode"`
		PromptWAV   string `json:"prompt_wav"`
		PromptText  string `json:"prompt_text"`
		Instruction string `json:"instruction"`
	} `json:"tts"`
}

func LoadConfig(root config.Config) (Config, error) {
	var c Config
	if err := root.DecodeApp(ID, &c); err != nil {
		return c, err
	}
	if c.DataDir == "" {
		c.DataDir = root.AppDataDir(ID)
	} else {
		c.DataDir = root.Resolve(c.DataDir)
	}
	if c.ExtensionsDir == "" {
		c.ExtensionsDir = root.Resolve("agent")
	} else {
		c.ExtensionsDir = root.Resolve(c.ExtensionsDir)
	}
	c.LLM.KeyFile = root.Resolve(c.LLM.KeyFile)
	c.ASR.KeyFile = root.Resolve(c.ASR.KeyFile)
	c.TTS.KeyFile = root.Resolve(c.TTS.KeyFile)
	c.TTS.PromptWAV = root.Resolve(c.TTS.PromptWAV)
	if c.LLM.Model == "" {
		return c, errors.New("缺少 llm.model")
	}
	for name, s := range map[string]config.Service{"llm": c.LLM.Service, "asr": c.ASR.Service, "tts": c.TTS.Service} {
		if err := s.Validate(name, name == "asr"); err != nil {
			return c, err
		}
	}
	if c.ASR.Language == "" {
		c.ASR.Language = "中文"
	}
	if c.TTS.Mode == "" {
		c.TTS.Mode = "instruct2"
	}
	if c.TTS.Mode != "instruct2" && c.TTS.Mode != "zero_shot" && c.TTS.Mode != "cross_lingual" {
		return c, errors.New("不支持的 tts.mode")
	}
	if c.TTS.Mode == "zero_shot" && c.TTS.PromptText == "" {
		return c, errors.New("zero_shot 需要参考音频的准确转写 prompt_text")
	}
	return c, nil
}
