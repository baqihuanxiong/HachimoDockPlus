// Package system manages the board's staged configuration and system services.
package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"hachimodockplus/internal/agent/profile"
	"hachimodockplus/internal/filetree"
)

// Validator checks a snapshot using the installed configuration's path base.
// secretDir contains the snapshot's private copies, not the currently live keys.
type Validator func(data []byte, secretDir string) error

type bundle struct {
	config []byte
	keys   map[string][]byte
	agent  map[string][]byte
	assets map[string][]byte
}

func regularFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ls, err := os.Lstat(path)
	if err != nil || !ls.Mode().IsRegular() {
		return nil, errors.New("配置中不能包含链接或特殊文件")
	}
	st, err := f.Stat()
	if err != nil || !os.SameFile(ls, st) || st.Size() > limit {
		return nil, errors.New("配置文件已变化或过大")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != st.Size() {
		return nil, errors.New("配置仍在写入，请稍后重试")
	}
	return b, nil
}

func readBundle(directory string) (bundle, error) {
	b := bundle{keys: make(map[string][]byte)}
	var err error
	b.config, err = regularFile(filepath.Join(directory, "config.json"), 1<<20)
	if err != nil {
		return b, errors.New("无法读取待应用的 config.json")
	}
	secretDir := filepath.Join(directory, "secrets")
	st, err := os.Lstat(secretDir)
	if err != nil || !st.IsDir() {
		return b, errors.New("secrets 必须是普通目录")
	}
	entries, err := os.ReadDir(secretDir)
	if err != nil || len(entries) > 64 {
		return b, errors.New("无法读取密钥目录或文件过多")
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		} // Desktop editor scratch files.
		data, err := regularFile(filepath.Join(secretDir, entry.Name()), 64<<10)
		if err != nil {
			return b, fmt.Errorf("密钥文件 %s 无效", entry.Name())
		}
		b.keys[entry.Name()] = data
	}
	b.assets, err = filetree.Read(filepath.Join(directory, "assets"))
	if err != nil {
		return b, err
	}
	b.agent, err = profile.Read(filepath.Join(directory, "agent"))
	if err != nil {
		return b, err
	}
	return b, nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func atomicLink(path, target string) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".link-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	defer os.Remove(name)
	if err = os.Symlink(target, name); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func writeBundle(directory string, b bundle) error {
	if err := os.MkdirAll(filepath.Join(directory, "secrets"), 0700); err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(directory, "config.json"), b.config, 0600); err != nil {
		return err
	}
	for name, data := range b.keys {
		if err := atomicWrite(filepath.Join(directory, "secrets", name), data, 0600); err != nil {
			return err
		}
	}
	if err := filetree.Write(filepath.Join(directory, "assets"), b.assets, nil); err != nil {
		return err
	}
	return profile.Write(filepath.Join(directory, "agent"), b.agent)
}

// PrepareConfiguration is run once by the installer while the Go UI is stopped.
// Runtime paths stay stable; a single symlink switches JSON and keys together.
func PrepareConfiguration(configPath string) error {
	base := filepath.Dir(configPath)
	systemDir := filepath.Join(base, "system")
	versions := filepath.Join(systemDir, "config-versions")
	if err := os.MkdirAll(versions, 0700); err != nil {
		return err
	}
	active := filepath.Join(systemDir, "config-active")
	if _, err := os.Readlink(active); errors.Is(err, os.ErrNotExist) {
		data, err := regularFile(configPath, 1<<20)
		if err != nil {
			return err
		}
		keys := map[string][]byte{}
		if entries, err := os.ReadDir(filepath.Join(base, "secrets")); err == nil {
			for _, entry := range entries {
				value, err := regularFile(filepath.Join(base, "secrets", entry.Name()), 64<<10)
				if err != nil {
					return err
				}
				keys[entry.Name()] = value
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		generation, err := os.MkdirTemp(versions, "cfg-")
		if err != nil {
			return err
		}
		agentFiles, err := profile.Read(filepath.Join(base, "agent"))
		if err != nil {
			os.RemoveAll(generation)
			return err
		}
		assets, err := filetree.Read(filepath.Join(base, "assets"))
		if err != nil {
			os.RemoveAll(generation)
			return err
		}
		if err = writeBundle(generation, bundle{config: data, keys: keys, agent: agentFiles, assets: assets}); err != nil {
			os.RemoveAll(generation)
			return err
		}
		relative, _ := filepath.Rel(systemDir, generation)
		if err = atomicLink(active, relative); err != nil {
			os.RemoveAll(generation)
			return err
		}
	} else if err != nil {
		return errors.New("生效配置目录无效")
	}
	if err := prepareAssets(base, active); err != nil {
		return err
	}
	// A pre-existing standalone profile is migrated before replacing its path
	// with the stable link. Refuse conflicting copies instead of deleting edits.
	if st, err := os.Lstat(filepath.Join(base, "agent")); err == nil && st.IsDir() {
		incoming, err := profile.Read(filepath.Join(base, "agent"))
		if err != nil {
			return err
		}
		current, err := profile.Read(filepath.Join(active, "agent"))
		if err != nil {
			return err
		}
		for name, data := range incoming {
			if old, ok := current[name]; ok && !bytes.Equal(old, data) {
				return errors.New("agent 迁移发现冲突，请保留两个目录核对")
			}
			current[name] = data
		}
		if err := profile.Write(filepath.Join(active, "agent"), current); err != nil {
			return err
		}
	}
	if err := profile.Ensure(filepath.Join(active, "agent")); err != nil {
		return err
	}
	// The share is a candidate, so malformed remote edits never break startup.
	share := filepath.Join(base, "config-share")
	if _, err := os.Stat(filepath.Join(share, "config.json")); errors.Is(err, os.ErrNotExist) {
		b, err := readBundle(active)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(share, 0700); err != nil {
			return err
		}
		if err = writeBundle(share, b); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	readme := "在 config.json 的 apps 下编辑组件，audio 下设置全局音量与 speaker/spdif/bluetooth 输出。secrets 中编辑服务密钥。\nagent/runtime.json 配置通用工具和 MCP；agent/AGENT.md、SOUL.md 配置智能体；agent/skills/ 放技能，agent/bin/ 放 ARMv7 CLI 或脚本。\n保存后到 设置 → 系统 → 应用配置并重启。校验失败时继续使用原配置。\nassets/ 可替换参考音频、字体等资源，与配置一起校验应用。\n相对路径仍以 /opt/hachimodock 为基准。会话及 CLI 私有运行数据不共享。\n"
	if err := atomicWrite(filepath.Join(share, "说明.txt"), []byte(readme), 0600); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(share, "assets")); errors.Is(err, os.ErrNotExist) {
		files, err := filetree.Read(filepath.Join(active, "assets"))
		if err != nil {
			return err
		}
		if err = filetree.Write(filepath.Join(share, "assets"), files, nil); err != nil {
			return err
		}
	}
	// Upgrade existing shares without replacing pending edits.
	if _, err := os.Lstat(filepath.Join(share, "agent")); errors.Is(err, os.ErrNotExist) {
		files, err := profile.Read(filepath.Join(active, "agent"))
		if err != nil {
			return err
		}
		if err := profile.Write(filepath.Join(share, "agent"), files); err != nil {
			return err
		}
	}
	if err := profile.Ensure(filepath.Join(share, "agent")); err != nil {
		return err
	}
	agentPath := filepath.Join(base, "agent")
	if st, err := os.Lstat(agentPath); err == nil && st.IsDir() {
		// Content was copied into the first generation above.
		if err := os.RemoveAll(agentPath); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := atomicLink(agentPath, "system/config-active/agent"); err != nil {
		return err
	}
	if err := atomicLink(configPath, "system/config-active/config.json"); err != nil {
		return err
	}
	secrets := filepath.Join(base, "secrets")
	if st, err := os.Lstat(secrets); err == nil && st.IsDir() {
		old := filepath.Join(systemDir, "secrets-migration")
		if _, err := os.Lstat(old); !errors.Is(err, os.ErrNotExist) {
			return errors.New("存在未完成的密钥迁移，请检查")
		}
		if err := os.Rename(secrets, old); err != nil {
			return err
		}
		if err := atomicLink(secrets, "system/config-active/secrets"); err != nil {
			_ = os.Rename(old, secrets)
			return err
		}
		return os.RemoveAll(old)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return atomicLink(secrets, "system/config-active/secrets")
}

func ApplyConfiguration(ctx context.Context, configPath string, validate Validator) error {
	base := filepath.Dir(configPath)
	share := filepath.Join(base, "config-share")
	b, err := readBundle(share)
	if err != nil {
		return err
	}
	versions := filepath.Join(base, "system", "config-versions")
	generation, err := os.MkdirTemp(versions, "cfg-")
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(generation)
		}
	}()
	if err = writeBundle(generation, b); err != nil {
		return err
	}
	for _, directory := range []string{filepath.Join(generation, "secrets"), filepath.Join(generation, "agent"), filepath.Join(generation, "assets"), generation, versions} {
		if err = syncDirectory(directory); err != nil {
			return err
		}
	}
	if validate == nil {
		return errors.New("配置校验器不可用")
	}
	if err = validate(b.config, filepath.Join(generation, "secrets")); err != nil {
		return err
	}
	current, err := readBundle(share)
	if err != nil || !bytes.Equal(b.config, current.config) || len(b.keys) != len(current.keys) || !profile.Equal(b.agent, current.agent) || !filetree.Equal(b.assets, current.assets) {
		return errors.New("配置仍在写入，请保存后重试")
	}
	for name, data := range b.keys {
		if !bytes.Equal(data, current.keys[name]) {
			return errors.New("密钥仍在写入，请保存后重试")
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	relative, _ := filepath.Rel(filepath.Join(base, "system"), generation)
	if err = atomicLink(filepath.Join(base, "system", "config-active"), relative); err != nil {
		return err
	}
	committed = true
	if syncDirectory(filepath.Join(base, "system")) != nil {
		return nil
	} // Keep the old generation if durability cannot be confirmed.
	// Retain only the active generation, not a series of old config backups.
	entries, _ := os.ReadDir(versions)
	for _, entry := range entries {
		path := filepath.Join(versions, entry.Name())
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "cfg-") && path != generation {
			_ = os.RemoveAll(path)
		}
	}
	return nil
}
