// Package standby owns idle and long-press policy; the system stays running.
package standby

import (
	"time"

	"hachimodockplus/internal/platform/controls"
)

const LongPress = 1500 * time.Millisecond

type Power interface{ SetOff(bool) error }
type Transition int

const (
	Unchanged Transition = iota
	Entered
	TaskWake
)

type Controller struct {
	power                        Power
	Off                          bool
	lastActivity, sw3Down, retry time.Time
	long                         bool
	swallowed                    map[controls.Key]bool
	pressed                      map[controls.Key]bool
	task                         string
}

func New(power Power, now time.Time) *Controller {
	return &Controller{power: power, lastActivity: now, swallowed: make(map[controls.Key]bool), pressed: make(map[controls.Key]bool)}
}

func (c *Controller) Activity(now time.Time) { c.lastActivity = now }

func (c *Controller) Wake(now time.Time) error {
	if c.Off {
		if err := c.power.SetOff(false); err != nil {
			return err
		}
		c.Off = false
	}
	c.Activity(now)
	c.sw3Down = time.Time{}
	c.long = false
	return nil
}

// Input consumes wake keys through release and separates SW3 short/long presses.
func (c *Controller) Input(e controls.Event, now time.Time) (shortSW3, consumed bool, err error) {
	c.Activity(now)
	wasPressed := c.pressed[e.Key]
	if e.Key <= controls.JoystickPress {
		if e.Pressed {
			c.pressed[e.Key] = true
		} else {
			delete(c.pressed, e.Key)
		}
	}
	if c.swallowed[e.Key] {
		if !e.Pressed {
			delete(c.swallowed, e.Key)
		}
		return false, true, nil
	}
	if c.Off {
		// A key held before manual standby still owns its release (e.g.
		// SW1 must finish voice recording). Wake-key releases are swallowed
		// above, and the SW3 long-press release never becomes navigation.
		if !e.Pressed && wasPressed && e.Key != controls.SW3 {
			return false, false, nil
		}
		if e.Pressed {
			err = c.Wake(now)
			if e.Key <= controls.JoystickPress {
				c.swallowed[e.Key] = true
			}
		}
		return false, true, err
	}
	if e.Key != controls.SW3 {
		return false, false, nil
	}
	if e.Pressed {
		if c.sw3Down.IsZero() {
			c.sw3Down = now
		}
	} else {
		shortSW3 = !c.sw3Down.IsZero() && !c.long
		c.sw3Down = time.Time{}
		c.long = false
	}
	return shortSW3, true, nil
}

// task is an identity/transition token, empty for stale, hidden or idle tasks.
// A repeated task cannot immediately undo a user's manual standby request.
func (c *Controller) Update(now time.Time, timeout time.Duration, task string) (Transition, error) {
	if c.power == nil {
		return Unchanged, nil
	}
	if c.Off {
		if task != "" && task != c.task {
			if now.Before(c.retry) {
				return Unchanged, nil
			}
			if err := c.Wake(now); err != nil {
				c.retry = now.Add(time.Second)
				return Unchanged, err
			}
			c.task = task
			return TaskWake, nil
		}
		c.task = task
		return Unchanged, nil
	}
	c.task = task
	held := !c.sw3Down.IsZero()
	manual := held && !c.long && now.Sub(c.sw3Down) >= LongPress
	if task != "" {
		c.Activity(now)
	}
	idle := timeout > 0 && len(c.pressed) == 0 && now.Sub(c.lastActivity) >= timeout
	if !manual && !idle || now.Before(c.retry) {
		return Unchanged, nil
	}
	if manual {
		c.long = true
	}
	if err := c.power.SetOff(true); err != nil {
		c.retry = now.Add(30 * time.Second)
		return Unchanged, err
	}
	c.Off = true
	return Entered, nil
}
