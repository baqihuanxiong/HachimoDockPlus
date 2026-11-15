// Package filetree snapshots bounded trees of configuration resources.
package filetree

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const MaxFileBytes = 16 << 20
const MaxTotalBytes = 32 << 20
const MaxFiles = 512

// Read takes a bounded snapshot. Symlinks, special files and in-progress writes
// are rejected, including directory links. Editor temporary files are ignored.
func Read(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	st, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return files, nil
	}
	if err != nil || !st.IsDir() {
		return nil, errors.New("资源必须是普通目录")
	}
	total, entries := 0, 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entries++
		if entries > MaxFiles || strings.Count(rel, string(filepath.Separator)) > 12 {
			return errors.New("资源文件过多或目录层级过深")
		}
		if d.IsDir() {
			return nil
		}
		st, err := os.Lstat(path)
		if err != nil || !st.Mode().IsRegular() || st.Size() > MaxFileBytes || st.Size() > int64(MaxTotalBytes-total) {
			return fmt.Errorf("资源文件 %s 无效、包含链接或超出大小限制", rel)
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		opened, err := f.Stat()
		if err != nil || !os.SameFile(st, opened) {
			f.Close()
			return errors.New("资源文件仍在变化")
		}
		data, err := io.ReadAll(io.LimitReader(f, st.Size()+1))
		f.Close()
		if err != nil {
			return err
		}
		if int64(len(data)) != st.Size() {
			return errors.New("资源文件仍在写入")
		}
		files[rel] = data
		total += len(data)
		return nil
	})
	return files, err
}

// Write restores a snapshot, optionally marking executable resources.
func Write(root string, files map[string][]byte, executable func(string, []byte) bool) error {
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	for name, data := range files {
		if name == "." || filepath.IsAbs(name) || filepath.Clean(name) != name || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return errors.New("资源文件路径无效")
		}
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		mode := os.FileMode(0600)
		if executable != nil && executable(name, data) {
			mode = 0700
		}
		if st, err := os.Lstat(path); err == nil && !st.Mode().IsRegular() {
			return errors.New("资源目标文件不能是链接或特殊文件")
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			return err
		}
		if err = f.Chmod(mode); err == nil {
			_, err = f.Write(data)
		}
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		err = f.Sync()
		f.Close()
		return err
	})
}

func Equal(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		other, ok := b[k]
		if !ok || !bytes.Equal(v, other) {
			return false
		}
	}
	return true
}
