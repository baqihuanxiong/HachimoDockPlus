// Package preferences persists user display settings independently of credentials.
package preferences

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type Theme struct {
	ID, Title string
	Color     uint32
}

var themes = []Theme{{"teal", "青绿", 0x0f766e}, {"blue", "蓝色", 0x2563eb}, {"violet", "紫色", 0x7c3aed}, {"amber", "琥珀", 0xd97706}, {"rose", "玫红", 0xdb2777}, {"cyan", "青色", 0x0891b2}, {"orange", "橙色", 0xea580c}, {"green", "绿色", 0x16a34a}, {"red", "红色", 0xdc2626}}

func Themes() []Theme { return append([]Theme(nil), themes...) }
func Find(id string) (Theme, bool) {
	for _, theme := range themes {
		if theme.ID == id {
			return theme, true
		}
	}
	return Theme{}, false
}

type Snapshot struct {
	StandbySeconds int    `json:"standby_seconds"`
	Screensaver    string `json:"screensaver"`
	Theme          string `json:"theme"`
	Mode           string `json:"mode"`
}
type Store struct {
	mu    sync.Mutex
	write sync.Mutex
	path  string
	state Snapshot
}

func Load(path string) (*Store, error) {
	s := &Store{path: path, state: Snapshot{Theme: "teal", Mode: "dark", Screensaver: "clock"}}
	if path == "" {
		return s, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(data, &s.state) != nil {
		return nil, errors.New("外观偏好格式错误")
	}
	if _, ok := Find(s.state.Theme); !ok {
		return nil, errors.New("外观偏好包含未知主题")
	}
	if s.state.Mode != "light" && s.state.Mode != "dark" {
		return nil, errors.New("外观偏好包含未知显示模式")
	}
	if !validScreensaver(s.state.Screensaver) {
		return nil, errors.New("偏好包含未知屏保")
	}
	if !validStandby(s.state.StandbySeconds) {
		return nil, errors.New("偏好包含无效待机时长")
	}
	return s, nil
}
func (s *Store) Snapshot() Snapshot { s.mu.Lock(); defer s.mu.Unlock(); return s.state }
func (s *Store) SetTheme(id string) error {
	if _, ok := Find(id); !ok {
		return errors.New("不支持此主题色")
	}
	return s.update(func(next *Snapshot) { next.Theme = id })
}
func (s *Store) SetMode(mode string) error {
	if mode != "light" && mode != "dark" {
		return errors.New("不支持此显示模式")
	}
	return s.update(func(next *Snapshot) { next.Mode = mode })
}
func (s *Store) SetScreensaver(id string) error {
	if !validScreensaver(id) {
		return errors.New("不支持此屏保")
	}
	return s.update(func(next *Snapshot) { next.Screensaver = id })
}
func validStandby(seconds int) bool {
	switch seconds {
	case 0, 30, 60, 180, 300, 600:
		return true
	}
	return false
}
func (s *Store) SetStandby(seconds int) error {
	if !validStandby(seconds) {
		return errors.New("不支持此待机时长")
	}
	return s.update(func(next *Snapshot) { next.StandbySeconds = seconds })
}
func validScreensaver(id string) bool { return id == "clock" || id == "blank" || id == "pet" }
func (s *Store) update(change func(*Snapshot)) error {
	s.write.Lock()
	defer s.write.Unlock()
	next := s.Snapshot()
	change(&next)
	if s.path != "" {
		dir := filepath.Dir(s.path)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		f, err := os.CreateTemp(dir, ".preferences-*")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		if err = json.NewEncoder(f).Encode(next); err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		if err = os.Rename(f.Name(), s.path); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.state = next
	s.mu.Unlock()
	return nil
}
