package system

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"hachimodockplus/internal/filetree"
)

// Upgrade pre-assets installations without overwriting a pending SMB candidate.
func prepareAssets(base, active string) error {
	path := filepath.Join(base, "assets")
	st, err := os.Lstat(path)
	if err == nil && st.IsDir() {
		incoming, err := filetree.Read(path)
		if err != nil {
			return err
		}
		current, err := filetree.Read(filepath.Join(active, "assets"))
		if err != nil {
			return err
		}
		for name, data := range incoming {
			if old, ok := current[name]; ok && !bytes.Equal(old, data) {
				return errors.New("assets 迁移发现冲突，请核对资源目录")
			}
			current[name] = data
		}
		if err = filetree.Write(filepath.Join(active, "assets"), current, nil); err != nil {
			return err
		}
		if err = os.RemoveAll(path); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err = os.MkdirAll(filepath.Join(active, "assets"), 0700); err != nil {
		return err
	}
	return atomicLink(path, "system/config-active/assets")
}

// CandidatePath resolves a live resource against the snapshot being validated.
func CandidatePath(base, snapshot, path string) string {
	rel, err := filepath.Rel(filepath.Join(base, "assets"), path)
	if err == nil && rel != "." && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Join(snapshot, "assets", rel)
	}
	return path
}
