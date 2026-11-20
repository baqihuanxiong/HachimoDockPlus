package standby

import (
	"errors"
	"testing"
	"time"

	"hachimodockplus/internal/platform/controls"
)

type fakePower struct {
	calls []bool
	err   error
}

func (p *fakePower) SetOff(off bool) error { p.calls = append(p.calls, off); return p.err }

func TestIdleActivityAndTaskKeepAlive(t *testing.T) {
	now := time.Now()
	p := &fakePower{}
	c := New(p, now)
	c.Update(now.Add(time.Hour), 0, "")
	if c.Off {
		t.Fatal("disabled timer entered standby")
	}
	c.Activity(now.Add(time.Hour))
	c.Update(now.Add(time.Hour+29*time.Second), 30*time.Second, "")
	if c.Off {
		t.Fatal("timer fired early")
	}
	c.Update(now.Add(time.Hour+30*time.Second), 30*time.Second, "codex/a/1/working")
	c.Update(now.Add(2*time.Hour), 30*time.Second, "codex/a/1/working")
	if c.Off {
		t.Fatal("active task was blanked")
	}
	c.Update(now.Add(2*time.Hour+29*time.Second), 30*time.Second, "")
	tr, err := c.Update(now.Add(2*time.Hour+30*time.Second), 30*time.Second, "")
	if err != nil || tr != Entered || !c.Off {
		t.Fatal(tr, err, c.Off)
	}
}

func TestShortAndLongSW3AndWakeRelease(t *testing.T) {
	now := time.Now()
	p := &fakePower{}
	c := New(p, now)
	c.Input(controls.Event{Key: controls.SW3, Pressed: true}, now)
	short, consumed, _ := c.Input(controls.Event{Key: controls.SW3}, now.Add(time.Second))
	if !short || !consumed || len(p.calls) != 0 {
		t.Fatal("short press changed power")
	}
	c.Input(controls.Event{Key: controls.SW3, Pressed: true}, now.Add(2*time.Second))
	c.Update(now.Add(2*time.Second+LongPress-time.Millisecond), 0, "")
	if c.Off {
		t.Fatal("long press fired early")
	}
	c.Update(now.Add(2*time.Second+LongPress), 0, "")
	if !c.Off {
		t.Fatal("long press did not blank")
	}
	short, consumed, _ = c.Input(controls.Event{Key: controls.SW3}, now.Add(4*time.Second))
	if short || !consumed || !c.Off {
		t.Fatal("long release navigated or woke")
	}
	for _, key := range []controls.Key{controls.SW1, controls.SW2, controls.SW3, controls.JoystickPress, controls.Up} {
		c.Off = true
		short, consumed, err := c.Input(controls.Event{Key: key, Pressed: true}, now.Add(5*time.Second))
		if err != nil || short || !consumed || c.Off {
			t.Fatal(key, short, consumed, err)
		}
		if key <= controls.JoystickPress {
			short, consumed, _ = c.Input(controls.Event{Key: key}, now.Add(6*time.Second))
			if short || !consumed {
				t.Fatal("wake release leaked to application", key)
			}
		}
	}
}

func TestNewTaskWakesButRepeatedTaskDoesNot(t *testing.T) {
	now := time.Now()
	c := New(&fakePower{}, now)
	c.Update(now, 0, "codex/a/1/working")
	c.Input(controls.Event{Key: controls.SW3, Pressed: true}, now)
	c.Update(now.Add(LongPress), 0, "codex/a/1/working")
	tr, _ := c.Update(now.Add(2*time.Second), 0, "codex/a/1/working")
	if tr != Unchanged || !c.Off {
		t.Fatal("repeat undid manual standby")
	}
	tr, err := c.Update(now.Add(3*time.Second), 0, "codex/a/2/working")
	if err != nil || tr != TaskWake || c.Off {
		t.Fatal(tr, err)
	}
}

func TestKeyHeldBeforeStandbyCanFinishItsOperation(t *testing.T) {
	now := time.Now()
	c := New(&fakePower{}, now)
	c.Input(controls.Event{Key: controls.SW1, Pressed: true}, now)
	c.Update(now.Add(time.Minute), time.Second, "")
	if c.Off {
		t.Fatal("auto standby interrupted held recording key")
	}
	c.Input(controls.Event{Key: controls.SW3, Pressed: true}, now.Add(time.Minute))
	c.Update(now.Add(time.Minute+LongPress), time.Second, "")
	if !c.Off {
		t.Fatal("manual standby failed")
	}
	_, consumed, _ := c.Input(controls.Event{Key: controls.SW1}, now.Add(2*time.Minute))
	if consumed || !c.Off {
		t.Fatal("recording release lost or unexpectedly woke display")
	}
}

func TestPowerFailureAndWakeRetry(t *testing.T) {
	now := time.Now()
	p := &fakePower{err: errors.New("ioctl failed")}
	c := New(p, now)
	if _, err := c.Update(now.Add(time.Minute), time.Second, ""); err == nil || c.Off {
		t.Fatal("blank failure claimed success")
	}
	c.Update(now.Add(time.Minute+time.Second), time.Second, "")
	if len(p.calls) != 1 {
		t.Fatal("blank failure retried each tick")
	}
	p.err = nil
	c.Update(now.Add(2*time.Minute), time.Second, "")
	p.err = errors.New("wake failed")
	c.Update(now.Add(2*time.Minute+time.Second), time.Second, "codex/task/1/working")
	if !c.Off {
		t.Fatal("failed wake changed state")
	}
	p.err = nil
	tr, err := c.Update(now.Add(2*time.Minute+2*time.Second), time.Second, "codex/task/1/working")
	if err != nil || tr != TaskWake || c.Off {
		t.Fatal("pending wake was lost", tr, err)
	}
}
