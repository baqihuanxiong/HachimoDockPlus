package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

var audioWrite sync.Mutex

// SaveAudio merges only runtime audio preferences into JSON, retaining relative
// paths, app fields and pending SMB edits. Resolve links before atomic replacement.
func (c *Config) SaveAudio(volume int, output, address string) error {
	audioWrite.Lock()
	defer audioWrite.Unlock()
	if c.source == "" {
		return nil
	}
	updates := map[string]any{}
	if volume != c.Audio.Volume {
		updates["volume"] = volume
	}
	if output != c.Audio.Output {
		updates["output"] = output
	}
	if address != c.Audio.BluetoothDevice {
		updates["bluetooth_device"] = address
	}
	if len(updates) == 0 {
		return nil
	}
	if err := patchAudio(c.source, updates); err != nil {
		return err
	}
	share := filepath.Join(filepath.Dir(c.source), "config-share", "config.json")
	if _, err := os.Lstat(share); !errors.Is(err, os.ErrNotExist) {
		if err = patchAudio(share, updates); err != nil {
			return err
		}
	}
	c.Audio.Volume, c.Audio.Output, c.Audio.BluetoothDevice = volume, output, address
	return nil
}

func patchAudio(path string, updates map[string]any) error {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	original := data
	var root map[string]json.RawMessage
	if err = json.Unmarshal(data, &root); err != nil || root == nil {
		return errors.New("无法保存音频设置：config.json 格式错误")
	}
	fields := map[string]json.RawMessage{}
	if raw, ok := root["audio"]; ok {
		if err = json.Unmarshal(raw, &fields); err != nil || fields == nil {
			return errors.New("无法保存音频设置：audio 格式错误")
		}
	}
	for key, value := range updates {
		fields[key], err = json.Marshal(value)
		if err != nil {
			return err
		}
	}
	root["audio"], err = json.Marshal(fields)
	if err != nil {
		return err
	}
	data, err = json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".audio-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(data, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	current, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(original, current) {
		return errors.New("config.json 仍在写入，请稍后重试")
	}
	if err = os.Rename(f.Name(), target); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(target))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// InitializeAudio upgrades legacy volume.json and seeds missing audio fields in
// each document independently, preserving any explicit candidate preferences.
func (c *Config) InitializeAudio() error {
	audioWrite.Lock()
	defer audioWrite.Unlock()
	if c.source == "" {
		return nil
	}
	defaults := map[string]any{"volume": c.Audio.Volume, "output": c.Audio.Output, "bluetooth_device": c.Audio.BluetoothDevice, "spdif_device": c.Audio.SPDIFDevice}
	paths := []string{c.source, filepath.Join(filepath.Dir(c.source), "config-share", "config.json")}
	for i, path := range paths {
		data, err := os.ReadFile(path)
		if i > 0 && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		var document struct {
			Audio map[string]json.RawMessage `json:"audio"`
		}
		if json.Unmarshal(data, &document) != nil {
			if i > 0 {
				continue
			} // An invalid candidate must not prevent startup.
			return errors.New("config.json 格式错误")
		}
		missing := map[string]any{}
		for key, value := range defaults {
			if _, ok := document.Audio[key]; !ok {
				missing[key] = value
			}
		}
		if len(missing) > 0 {
			if err = patchAudio(path, missing); err != nil {
				if i == 0 {
					return err
				}
			}
		}
	}
	c.volumeSet = true
	// Only the config JSON is read on later starts. Discard the obsolete preference.
	if err := os.Remove(filepath.Join(c.DataDir, "volume.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
