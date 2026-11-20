// Package pet implements local desktop-pet media and presentation state.
package pet

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var familyID = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

// Pack is intentionally independent of the desktop's private manifest paths.
// An offline pack contains videos/<family>.mp4, or flat <family>.mp4 files.
type Pack struct {
	Name  string
	Clips map[string]string
}

func LoadPack(directory string) (Pack, error) {
	p := Pack{Clips: make(map[string]string), Name: "桌宠"}
	root, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return p, errors.New("桌宠素材尚未导入")
	}
	st, err := os.Lstat(root)
	if err != nil || !st.IsDir() {
		return p, errors.New("桌宠素材目录无效")
	}
	// manifest.json is optional. Source videoPath values are deliberately ignored:
	// official exports can contain desktop-local absolute paths and credentials.
	manifest := filepath.Join(root, "manifest.json")
	if info, err := os.Lstat(manifest); err == nil && (!info.Mode().IsRegular() || info.Size() > 1<<20) {
		return p, errors.New("桌宠清单不能是链接且必须小于 1 MiB")
	}
	if f, err := os.Open(manifest); err == nil {
		var meta struct {
			Name           string `json:"name"`
			AppearanceName string `json:"appearanceName"`
		}
		err = json.NewDecoder(io.LimitReader(f, 1<<20)).Decode(&meta)
		f.Close()
		if err != nil {
			return p, errors.New("桌宠清单格式错误")
		}
		if meta.Name != "" {
			p.Name = cleanText(meta.Name, 96)
		} else if meta.AppearanceName != "" {
			p.Name = cleanText(meta.AppearanceName, 96)
		}
	}
	for _, sub := range []string{"", "videos"} {
		dir := filepath.Join(root, sub)
		st, err := os.Lstat(dir)
		if sub != "" && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !st.IsDir() {
			return p, errors.New("桌宠视频目录不能是链接")
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return p, err
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") || !strings.EqualFold(filepath.Ext(entry.Name()), ".mp4") {
				continue
			}
			family := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			if !familyID.MatchString(family) {
				return p, errors.New("桌宠动作文件名无效")
			}
			file := filepath.Join(dir, entry.Name())
			st, err := os.Lstat(file)
			if err != nil || !st.Mode().IsRegular() || st.Size() == 0 || st.Size() > 16<<20 {
				return p, fmt.Errorf("桌宠动作 %s 无效或超过 16 MiB", family)
			}
			if _, exists := p.Clips[family]; exists {
				return p, fmt.Errorf("桌宠动作 %s 重复", family)
			}
			p.Clips[family] = file
			if len(p.Clips) > 64 {
				return p, errors.New("桌宠动作不能超过 64 个")
			}
		}
	}
	if len(p.Clips) == 0 {
		return p, errors.New("素材目录中没有 MP4 动作")
	}
	return p, nil
}

func (p Pack) Clip(state string, variation uint64) (string, string) {
	var candidates []string
	for family := range p.Clips {
		if family == state || (state == "working" && strings.HasPrefix(family, "working.")) || (state == "idle" && strings.HasPrefix(family, "idle.")) {
			candidates = append(candidates, family)
		}
	}
	if len(candidates) == 0 {
		for _, fallback := range []string{"idle.default", "idle.wandering", "idle.playing", "welcome"} {
			if file := p.Clips[fallback]; file != "" {
				return fallback, file
			}
		}
		for family := range p.Clips {
			candidates = append(candidates, family)
		}
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return "", ""
	}
	family := candidates[variation%uint64(len(candidates))]
	return family, p.Clips[family]
}

// TouchClip follows the official pet's thirds-of-screen selection. A pack
// without touch clips keeps its current animation instead of falling back to
// an unrelated lifecycle clip.
func (p Pack) TouchClip(x int, variation uint64) (string, string) {
	directional := ""
	if x < 214 {
		directional = "touch.left"
	} else if x > 426 {
		directional = "touch.right"
	}
	if file := p.Clips[directional]; directional != "" && file != "" {
		return directional, file
	}
	families := []string{"touch.lick", "touch.what"}
	for i := uint64(0); i < 2; i++ {
		family := families[(variation+i)%2]
		if file := p.Clips[family]; file != "" {
			return family, file
		}
	}
	return "", ""
}
