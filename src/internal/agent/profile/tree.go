package profile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"hachimodockplus/internal/filetree"
)

const (
	MaxFileBytes  = filetree.MaxFileBytes
	MaxTotalBytes = filetree.MaxTotalBytes
	MaxFiles      = filetree.MaxFiles
)

func Read(root string) (map[string][]byte, error) { return filetree.Read(root) }
func Equal(a, b map[string][]byte) bool           { return filetree.Equal(a, b) }
func Write(root string, files map[string][]byte) error {
	return filetree.Write(root, files, func(name string, data []byte) bool {
		return strings.HasPrefix(filepath.ToSlash(name), "bin/") || bytes.HasPrefix(data, []byte("#!"))
	})
}

// Ensure seeds a new profile; an existing runtime.json marks user-owned content.
func Ensure(root string) error {
	if _, err := os.Stat(filepath.Join(root, "runtime.json")); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	files, err := Read(root)
	if err != nil {
		return err
	}
	for name, data := range Defaults() {
		if _, exists := files[name]; !exists {
			files[name] = data
		}
	}
	return Write(root, files)
}
