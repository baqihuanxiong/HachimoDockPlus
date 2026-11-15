// Package config loads shell configuration; app schemas are decoded on demand.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"hachimodockplus/internal/platform/controls"
)

type Service struct {
	URL     string `json:"url"`
	KeyFile string `json:"key_file"`
}

var bluetoothAddress = regexp.MustCompile(`(?i)^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)

type Audio struct {
	CaptureDevice   string `json:"capture_device"`
	PlaybackDevice  string `json:"playback_device"`
	MixerCard       string `json:"mixer_card"`
	Volume          int    `json:"volume"`
	Output          string `json:"output"`
	SPDIFDevice     string `json:"spdif_device"`
	BluetoothDevice string `json:"bluetooth_device"`
}
type UI struct {
	AppOrder    []string         `json:"app_order,omitempty"`
	Controls    controls.Options `json:"controls"`
	Timezone    string           `json:"timezone"`
	Framebuffer string           `json:"framebuffer"`
	Input       string           `json:"input"`
	Font        string           `json:"font"`
}
type Config struct {
	DataDir   string                     `json:"data_dir"`
	Audio     Audio                      `json:"audio"`
	UI        UI                         `json:"ui"`
	Apps      map[string]json.RawMessage `json:"apps"`
	base      string
	source    string
	volumeSet bool
}

func decode(data []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("配置必须只包含一个 JSON 对象")
	}
	return nil
}
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("读取配置: %w", err)
	}
	c, err := Parse(data, filepath.Dir(path))
	if err != nil {
		return c, err
	}
	c.source, err = filepath.Abs(path)
	if err != nil {
		return c, err
	}
	if !c.volumeSet {
		legacy, e := os.ReadFile(filepath.Join(c.DataDir, "volume.json"))
		if e == nil {
			var saved struct {
				Volume *int `json:"volume"`
			}
			if json.Unmarshal(legacy, &saved) != nil || saved.Volume == nil || *saved.Volume < 0 || *saved.Volume > 100 {
				return c, errors.New("旧音量偏好格式错误")
			}
			c.Audio.Volume = *saved.Volume
		} else if !errors.Is(e, os.ErrNotExist) {
			return c, e
		}
	}
	return c, nil
}

// SourcePath identifies the stable configuration entry used by the supervisor.
func (c Config) SourcePath() string { return c.source }

// Parse validates a candidate while retaining the installed config's path base.
func Parse(data []byte, baseDir string) (Config, error) {
	var c Config
	// Read legacy fields only to translate them to the app namespace in memory.
	var wire struct {
		DataDir string                     `json:"data_dir"`
		Audio   Audio                      `json:"audio"`
		UI      UI                         `json:"ui"`
		Apps    map[string]json.RawMessage `json:"apps"`
		LLM     json.RawMessage            `json:"llm"`
		ASR     json.RawMessage            `json:"asr"`
		TTS     json.RawMessage            `json:"tts"`
	}
	wire.Audio.Volume = 60
	if err := decode(data, &wire); err != nil {
		return c, fmt.Errorf("配置格式错误: %w", err)
	}
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return c, err
	}
	var presence struct {
		Audio map[string]json.RawMessage `json:"audio"`
	}
	_ = json.Unmarshal(data, &presence)
	_, volumeSet := presence.Audio["volume"]
	c = Config{volumeSet: volumeSet, DataDir: wire.DataDir, Audio: wire.Audio, UI: wire.UI, Apps: wire.Apps, base: base}
	if c.Apps == nil {
		c.Apps = make(map[string]json.RawMessage)
	}
	if len(wire.LLM)+len(wire.ASR)+len(wire.TTS) > 0 {
		if _, exists := c.Apps["picoclaw"]; exists {
			return c, errors.New("不能同时配置旧版服务字段和 apps.picoclaw")
		}
		legacy := map[string]any{"data_dir": wire.DataDir}
		for k, v := range map[string]json.RawMessage{"llm": wire.LLM, "asr": wire.ASR, "tts": wire.TTS} {
			if len(v) > 0 {
				legacy[k] = v
			}
		}
		c.Apps["picoclaw"], err = json.Marshal(legacy)
		if err != nil {
			return c, err
		}
	}
	if c.DataDir == "" {
		return c, errors.New("缺少 data_dir")
	}
	c.DataDir = c.Resolve(c.DataDir)
	c.UI.Font = c.Resolve(c.UI.Font)
	if c.Audio.CaptureDevice == "" {
		c.Audio.CaptureDevice = "default"
	}
	if c.Audio.PlaybackDevice == "" {
		c.Audio.PlaybackDevice = "default"
	}
	if c.Audio.Volume < 0 || c.Audio.Volume > 100 {
		return c, errors.New("audio.volume 必须为 0～100")
	}
	if c.Audio.Output == "" {
		c.Audio.Output = "speaker"
	}
	switch c.Audio.Output {
	case "speaker", "spdif", "bluetooth":
	default:
		return c, errors.New("audio.output 必须为 speaker、spdif 或 bluetooth")
	}
	if c.Audio.SPDIFDevice == "" {
		c.Audio.SPDIFDevice = "iec958:CARD=Device,DEV=0"
	}
	if c.Audio.BluetoothDevice != "" && !bluetoothAddress.MatchString(c.Audio.BluetoothDevice) {
		return c, errors.New("audio.bluetooth_device 必须为蓝牙 MAC 地址")
	}
	c.Audio.BluetoothDevice = strings.ToUpper(c.Audio.BluetoothDevice)
	if c.Audio.Output == "bluetooth" && c.Audio.BluetoothDevice == "" {
		return c, errors.New("蓝牙输出需要 audio.bluetooth_device")
	}
	if c.UI.Framebuffer == "" {
		c.UI.Framebuffer = "/dev/fb0"
	}
	if c.UI.Timezone == "" {
		c.UI.Timezone = "Asia/Shanghai"
	}
	if c.UI.Input == "" {
		c.UI.Input = "auto"
	}
	return c, nil
}
func (c Config) Resolve(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(c.base, path)
}

// AppDataDir is for validated registry IDs; schemas may override it for migration.
func (c Config) AppDataDir(id string) string { return filepath.Join(c.DataDir, "apps", id) }
func (c Config) DecodeApp(id string, out any) error {
	raw, ok := c.Apps[id]
	if !ok {
		return fmt.Errorf("缺少 apps.%s 配置", id)
	}
	if err := decode(raw, out); err != nil {
		return fmt.Errorf("apps.%s 配置错误: %w", id, err)
	}
	return nil
}
func (s Service) Validate(name string, websocket bool) error {
	u, err := url.Parse(s.URL)
	valid := err == nil && (u.Scheme == "http" || u.Scheme == "https")
	if websocket {
		valid = err == nil && (u.Scheme == "ws" || u.Scheme == "wss")
	}
	if !valid || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%s.url 必须是无凭据、无查询参数的服务地址", name)
	}
	if s.KeyFile == "" {
		return fmt.Errorf("缺少 %s.key_file", name)
	}
	return nil
}
func (s Service) Key() (string, error) {
	st, err := os.Stat(s.KeyFile)
	if err != nil {
		return "", errors.New("无法读取服务密钥文件")
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return "", errors.New("密钥必须是仅当前用户可读的普通文件（chmod 600）")
	}
	b, err := os.ReadFile(s.KeyFile)
	if err != nil {
		return "", errors.New("无法读取服务密钥文件")
	}
	key := strings.TrimSpace(string(b))
	if key == "" || strings.ContainsAny(key, "\r\n") {
		return "", errors.New("密钥为空或包含换行")
	}
	return key, nil
}
