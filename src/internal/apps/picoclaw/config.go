package picoclaw

import (
	"errors"
	"strings"

	"hachimodockplus/internal/config"
	"hachimodockplus/internal/speech"
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
		Language   string `json:"language"`
		Provider   string `json:"provider,omitempty"`
		ResourceID string `json:"resource_id,omitempty"`
	} `json:"asr"`
	TTS struct {
		config.Service
		Mode        string `json:"mode"`
		PromptWAV   string `json:"prompt_wav"`
		PromptText  string `json:"prompt_text"`
		Instruction string `json:"instruction"`
		Provider    string `json:"provider,omitempty"`
		ResourceID  string `json:"resource_id,omitempty"`
		Speaker     string `json:"speaker,omitempty"`
		SpeechRate  int    `json:"speech_rate,omitempty"`
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
	if c.ASR.Provider == "" {
		c.ASR.Provider = "custom"
	}
	if c.TTS.Provider == "" {
		c.TTS.Provider = "custom"
	}
	for _, provider := range []string{c.ASR.Provider, c.TTS.Provider} {
		if provider != "custom" && provider != speech.ProviderVolcengine {
			return c, errors.New("语音 provider 必须为 custom 或 volcengine")
		}
	}
	if c.ASR.Provider == speech.ProviderVolcengine {
		if c.ASR.URL == "" {
			c.ASR.URL = speech.VolcengineASRURL
		}
		if c.ASR.ResourceID == "" {
			c.ASR.ResourceID = speech.VolcengineASRResource
		}
	}
	if c.TTS.Provider == speech.ProviderVolcengine {
		if c.TTS.URL == "" {
			c.TTS.URL = speech.VolcengineTTSURL
		}
		if c.TTS.ResourceID == "" {
			c.TTS.ResourceID = speech.VolcengineTTSResource
		}
		if c.TTS.Speaker == "" {
			c.TTS.Speaker = speech.VolcengineSpeaker
		}
		if c.TTS.SpeechRate < -50 || c.TTS.SpeechRate > 100 {
			return c, errors.New("tts.speech_rate 必须为 -50～100")
		}
	}
	for _, id := range []string{c.ASR.ResourceID, c.TTS.ResourceID} {
		if strings.ContainsAny(id, "\r\n") {
			return c, errors.New("语音资源 ID 不能包含换行")
		}
	}
	for name, s := range map[string]config.Service{"llm": c.LLM.Service, "asr": c.ASR.Service, "tts": c.TTS.Service} {
		if err := s.Validate(name, name == "asr"); err != nil {
			return c, err
		}
	}
	if c.ASR.Language == "" {
		c.ASR.Language = "中文"
	}
	if c.TTS.Provider == speech.ProviderVolcengine {
		return c, nil
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
