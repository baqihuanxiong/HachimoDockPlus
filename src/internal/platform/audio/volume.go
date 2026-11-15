package audio

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// 100% is the board's tested maximum (-8 dB), not the codec's maximum gain.
const MaxPCM = 29

type VolumeState struct {
	Volume          int
	Output          string
	BluetoothDevice string
	Error           string
	Revision        uint64
}
type Mixer interface {
	Apply(context.Context, int) error
}
type HardwareMixer struct{ Card string }

func (h HardwareMixer) Apply(ctx context.Context, volume int) error {
	card := h.Card
	if card == "" {
		card = "Device"
	}
	raw := (volume*MaxPCM + 50) / 100
	if err := exec.CommandContext(ctx, "amixer", "-q", "-c", card, "cset", "name=PCM Playback Volume", strconv.Itoa(raw)+","+strconv.Itoa(raw)).Run(); err != nil {
		return errors.New("无法设置音量，请检查 ALSA 混音器")
	}
	mode := "on"
	if volume == 0 {
		mode = "off"
	}
	if err := exec.CommandContext(ctx, "amixer", "-q", "-c", card, "cset", "name=PCM Playback Switch", mode).Run(); err != nil {
		return errors.New("无法设置扬声器静音")
	}
	return nil
}

type Volume struct {
	dirty     bool
	close     sync.Once
	mu        sync.Mutex
	apply     sync.Mutex
	persistMu sync.Mutex
	path      string
	mixer     Mixer
	state     VolumeState
	devices   OutputDevices
	persist   func(VolumeState) error
	wake      chan struct{}
	cancel    context.CancelFunc
	done      chan struct{}
}

func NewVolume(path string, mixer Mixer) (*Volume, error) {
	v := &Volume{path: path, mixer: mixer, state: VolumeState{Volume: 60, Revision: 1}, wake: make(chan struct{}, 1), done: make(chan struct{})}
	if b, err := os.ReadFile(path); err == nil {
		var saved struct {
			Volume *int `json:"volume"`
		}
		if json.Unmarshal(b, &saved) != nil || saved.Volume == nil || *saved.Volume < 0 || *saved.Volume > 100 {
			return nil, errors.New("音量偏好格式错误")
		}
		v.state.Volume = *saved.Volume
	} else if !errors.Is(err, os.ErrNotExist) && path != "" {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	go v.run(ctx)
	return v, nil
}
func (v *Volume) Snapshot() VolumeState { v.mu.Lock(); defer v.mu.Unlock(); return v.state }
func (v *Volume) Set(n int) {
	v.mu.Lock()
	v.dirty = true
	v.state.Volume = max(0, min(100, n))
	v.state.Revision++
	v.mu.Unlock()
	v.Initialize()
}
func (v *Volume) Adjust(delta int) {
	v.mu.Lock()
	v.dirty = true
	v.state.Volume = max(0, min(100, v.state.Volume+delta))
	v.state.Revision++
	v.mu.Unlock()
	v.Initialize()
}
func (v *Volume) Initialize() {
	select {
	case v.wake <- struct{}{}:
	default:
	}
}
func (v *Volume) Close() {
	v.close.Do(func() {
		v.cancel()
		<-v.done
		v.mu.Lock()
		dirty := v.dirty
		v.mu.Unlock()
		if dirty {
			_ = v.save()
		}
	})
}
func (v *Volume) Apply(ctx context.Context) error {
	v.apply.Lock()
	defer v.apply.Unlock()
	s := v.Snapshot()
	var err error
	if s.Output == "" || s.Output == "speaker" {
		err = v.mixer.Apply(ctx, s.Volume)
	}
	v.mu.Lock()
	if s.Revision == v.state.Revision {
		v.state.Error = ""
		if err != nil {
			v.state.Error = err.Error()
		}
		v.state.Revision++
	}
	v.mu.Unlock()
	return err
}
func (v *Volume) run(ctx context.Context) {
	defer close(v.done)
	for {
		select {
		case <-ctx.Done():
			return
		case <-v.wake:
			task, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := v.Apply(task)
			cancel()
			// Keep the requested preference even if the USB sound card is unplugged.
			if saveErr := v.save(); saveErr != nil {
				err = errors.New("音频设置已调整，但 config.json 保存失败")
			}
			if err != nil {
				v.mu.Lock()
				v.state.Error = err.Error()
				v.state.Revision++
				v.mu.Unlock()
			}
		}
	}
}

// Flush joins persistence before applying a staged configuration and restarting.
func (v *Volume) Flush() error { return v.save() }
func (v *Volume) save() error {
	v.persistMu.Lock()
	defer v.persistMu.Unlock()
	v.mu.Lock()
	s, dirty := v.state, v.dirty
	v.mu.Unlock()
	if !dirty {
		return nil
	}
	if err := v.saveState(s); err != nil {
		return err
	}
	v.mu.Lock()
	if v.state.Volume == s.Volume && v.state.Output == s.Output && v.state.BluetoothDevice == s.BluetoothDevice {
		v.dirty = false
	}
	v.mu.Unlock()
	return nil
}
func (v *Volume) saveState(s VolumeState) error {
	if v.persist != nil {
		return v.persist(s)
	}
	if v.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(v.path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(v.path), ".volume-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(struct {
		Volume int `json:"volume"`
	}{s.Volume})
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
	return os.Rename(f.Name(), v.path)
}

// The playback lock also joins a cancelled previous page before claiming ALSA.
var playbackLock sync.Mutex

func prepareOutput(ctx context.Context, volume *Volume) (func(), error) {
	playbackLock.Lock()
	release := func() { playbackLock.Unlock() }
	if ctx.Err() != nil {
		release()
		return nil, ctx.Err()
	}
	if volume != nil {
		if err := volume.Apply(ctx); err != nil {
			release()
			return nil, err
		}
	}
	if volume != nil && volume.configured() {
		if err := volume.route(ctx); err != nil {
			release()
			return nil, err
		}
		return func() { volume.releaseRoute(); playbackLock.Unlock() }, nil
	}
	if _, err := exec.LookPath("hachimodock-speaker-amp"); err == nil {
		if err = exec.CommandContext(ctx, "hachimodock-speaker-amp", "on").Run(); err != nil {
			release()
			return nil, errors.New("无法开启扬声器功放")
		}
		release = func() {
			clean, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = exec.CommandContext(clean, "hachimodock-speaker-amp", "off").Run()
			playbackLock.Unlock()
		}
	}
	return release, nil
}
