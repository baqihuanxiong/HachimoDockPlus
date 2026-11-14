package audio

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type testMixer struct {
	mu   sync.Mutex
	last int
}

func (m *testMixer) Apply(_ context.Context, n int) error {
	m.mu.Lock()
	m.last = n
	m.mu.Unlock()
	return nil
}
func TestVolumeClampAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "volume.json")
	m := &testMixer{}
	v, err := NewVolume(path, m)
	if err != nil {
		t.Fatal(err)
	}
	if v.Snapshot().Volume != 60 {
		t.Fatal("unexpected default")
	}
	v.Set(150)
	if v.Snapshot().Volume != 100 {
		t.Fatal("upper bound")
	}
	v.Adjust(-200)
	if v.Snapshot().Volume != 0 {
		t.Fatal("mute bound")
	}
	v.Set(59)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(path)
		if string(b) == "{\"volume\":59}\n" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	v.Close()
	restored, err := NewVolume(path, m)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if restored.Snapshot().Volume != 59 {
		t.Fatal("preference lost")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("permissions")
	}
}
