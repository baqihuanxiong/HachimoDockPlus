package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type credentials struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	ExpiresAt    int64  `json:"expires_at"`
}
type pendingLogin struct {
	DeviceID  string `json:"device_id"`
	State     string `json:"state"`
	ExpiresAt int64  `json:"expires_at"`
}
type device struct {
	DID    string `json:"did"`
	Name   string `json:"name"`
	Model  string `json:"model"`
	URN    string `json:"urn"`
	Home   string `json:"home"`
	Room   string `json:"room"`
	Online bool   `json:"online"`
}
type scene struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Home     string `json:"home"`
	HomeID   any    `json:"home_id"`
	OwnerUID any    `json:"owner_uid"`
	SceneID  any    `json:"scene_id"`
	RoomID   any    `json:"room_id"`
}
type cache struct {
	Devices []device        `json:"devices"`
	Scenes  []scene         `json:"scenes"`
	Specs   map[string]spec `json:"specs"`
}
type app struct {
	root  string
	cloud *cloudClient
	cache cache
	auth  credentials
}

func stateDirectory() (string, error) {
	if p := os.Getenv("MIJIA_STATE_DIR"); p != "" {
		return filepath.Abs(p)
	}
	if p := os.Getenv("HACHIMODOCK_CLI_STATE_DIR"); p != "" {
		return filepath.Abs(filepath.Join(p, "mijia"))
	}
	p, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(p, "mijia-lite"), nil
}

func openApp(ctx context.Context, root string) (*app, func(), error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, nil, err
	}
	st, err := os.Lstat(root)
	if err != nil || !st.IsDir() {
		return nil, nil, errors.New("私有状态路径不能是链接或特殊文件")
	}
	if err = os.Chmod(root, 0700); err != nil {
		return nil, nil, err
	}
	lockPath := filepath.Join(root, ".lock")
	fd, err := syscall.Open(lockPath, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, nil, err
	}
	lock := os.NewFile(uintptr(fd), lockPath)
	for {
		err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			lock.Close()
			return nil, nil, err
		}
		select {
		case <-ctx.Done():
			lock.Close()
			return nil, nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	closeFn := func() { syscall.Flock(fd, syscall.LOCK_UN); lock.Close() }
	a := &app{root: root, cloud: newCloudClient()}
	for name, target := range map[string]any{"credentials.json": &a.auth, "devices.json": &a.cache} {
		if err := a.read(name, target); err != nil && !errors.Is(err, os.ErrNotExist) {
			closeFn()
			return nil, nil, err
		}
	}
	if a.cache.Specs == nil {
		a.cache.Specs = map[string]spec{}
	}
	return a, closeFn, nil
}

func (a *app) read(name string, out any) error {
	path := filepath.Join(a.root, name)
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Size() > responseLimit || st.Mode().Perm()&0077 != 0 {
		return errors.New("私有状态文件权限或类型无效")
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, responseLimit+1))
	d.UseNumber()
	if err = d.Decode(out); err != nil {
		return errors.New("私有状态 JSON 损坏")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("私有状态 JSON 损坏")
	}
	return nil
}
func (a *app) write(name string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if len(b) > responseLimit {
		return errors.New("本地状态超出上限")
	}
	f, err := os.CreateTemp(a.root, ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
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
	return os.Rename(f.Name(), filepath.Join(a.root, name))
}

func (a *app) token(ctx context.Context) (string, error) {
	if a.auth.AccessToken == "" {
		return "", errors.New("请先运行 auth login 并完成授权")
	}
	if a.auth.ExpiresAt > time.Now().Unix()+60 {
		return a.auth.AccessToken, nil
	}
	next, err := a.cloud.token(ctx, map[string]any{"client_id": clientID, "redirect_uri": redirectURI, "refresh_token": a.auth.RefreshToken})
	if err != nil {
		return "", err
	}
	if err = a.write("credentials.json", next); err != nil {
		return "", err
	}
	a.auth = next
	return next.AccessToken, nil
}
func (a *app) post(ctx context.Context, path string, data any) (json.RawMessage, error) {
	token, err := a.token(ctx)
	if err != nil {
		return nil, err
	}
	return a.cloud.post(ctx, token, path, data)
}
