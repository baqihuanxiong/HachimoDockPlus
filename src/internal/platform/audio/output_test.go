package audio

import (
	"bytes"
	"encoding/binary"
	"io"
	"sync"
	"testing"
)

func TestConfiguredOutputPersistenceAndDigitalVolume(t *testing.T) {
	var mu sync.Mutex
	var saved []VolumeState
	v, err := NewConfiguredVolume(VolumeState{Volume: 50, Output: "speaker"}, OutputDevices{Speaker: "analog", SPDIF: "digital"}, &testMixer{}, func(s VolumeState) error { mu.Lock(); defer mu.Unlock(); saved = append(saved, s); return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	v.Initialize()
	if err = v.Flush(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	count := len(saved)
	mu.Unlock()
	if count != 0 {
		t.Fatal("initializing playback rewrote JSON")
	}
	if v.PlaybackDevice("") != "analog" {
		t.Fatal("speaker route")
	}
	if err = v.SetOutput("bluetooth", "invalid"); err == nil {
		t.Fatal("invalid address accepted")
	}
	if err = v.SetOutput("spdif", ""); err != nil {
		t.Fatal(err)
	}
	if v.PlaybackDevice("") != `plug:SLAVE="digital"` {
		t.Fatal("digital route")
	}
	samples := []int16{32767, -32768, 1000, -1000}
	var input bytes.Buffer
	for _, sample := range samples {
		binary.Write(&input, binary.LittleEndian, sample)
	}
	pcm := outputPCM(bytes.NewReader(input.Bytes()), v)
	// Exercise one-byte consumer reads as well as a short final source chunk.
	var output bytes.Buffer
	block := make([]byte, 1)
	for {
		n, e := pcm.Read(block)
		output.Write(block[:n])
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
	}
	for i, sample := range samples {
		if actual := int16(binary.LittleEndian.Uint16(output.Bytes()[i*2:])); actual != sample/2 {
			t.Fatal(actual, sample)
		}
	}
	v.Set(0)
	muted, err := io.ReadAll(outputPCM(bytes.NewReader(input.Bytes()), v))
	if err != nil || !bytes.Equal(muted, make([]byte, len(input.Bytes()))) {
		t.Fatal("digital mute failed", err)
	}
	if err = v.SetOutput("bluetooth", "aa:bb:cc:dd:ee:ff"); err != nil {
		t.Fatal(err)
	}
	if v.PlaybackDevice("") != `plug:SLAVE="bluealsa:DEV=AA:BB:CC:DD:EE:FF,PROFILE=a2dp"` {
		t.Fatal(v.PlaybackDevice(""))
	}
	if err = v.Flush(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	count = len(saved)
	mu.Unlock()
	if count == 0 {
		t.Fatal("preference not saved")
	}
	v.Close()
	mu.Lock()
	defer mu.Unlock()
	if len(saved) != count {
		t.Fatal("shutdown overwrote newly applied configuration")
	}
}
