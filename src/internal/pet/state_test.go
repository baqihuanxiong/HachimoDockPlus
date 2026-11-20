package pet

import (
	"fmt"
	"testing"
	"time"
)

func TestSelectedSessionOwnsMotionAndTerminalDeadline(t *testing.T) {
	var tracker Tracker
	now := time.Now()
	queue := func(state string, revision, remaining int) []byte {
		return []byte(fmt.Sprintf(`{"source":"codex","status":{"state":"working"},"sessions":{"agentId":"codex","sessionId":"a","displayEnabled":true,"sessions":[{"id":"a","title":"任务","state":%q,"transitionRevision":%d,"terminalRemainingMs":%d}]}}`, state, revision, remaining))
	}
	if err := tracker.Apply(queue("working", 1, 0), now); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Apply(queue("done", 2, 2000), now); err != nil {
		t.Fatal(err)
	}
	if tracker.View(now).State != "done" {
		t.Fatal("aggregate state overrode selection")
	}
	// A repeat and an older working revision cannot restart completion or undo it.
	tracker.Apply(queue("done", 2, 2000), now.Add(time.Second))
	tracker.Apply(queue("working", 1, 0), now.Add(1500*time.Millisecond))
	if tracker.View(now.Add(2500*time.Millisecond)).State != "idle" {
		t.Fatal("completion deadline extended")
	}
	tracker.Apply(queue("working", 3, 0), now.Add(3*time.Second))
	v := tracker.View(now.Add(3 * time.Second))
	if v.State != "working" || v.SessionID != "a" || v.TransitionRevision != 3 {
		t.Fatal("new transition lost")
	}
}

func TestProbeDoesNotKeepStalePresentationAndSourceSwitchClears(t *testing.T) {
	var tracker Tracker
	now := time.Now()
	if err := tracker.Apply([]byte(`{"source":"codex","status":{"state":"working","displayContent":"secret task"}}`), now); err != nil {
		t.Fatal(err)
	}
	tracker.Probe(now.Add(11 * time.Second))
	v := tracker.View(now.Add(11 * time.Second))
	if !v.Connected || v.State != "idle" || v.Content != "" {
		t.Fatal(v)
	}
	tracker.Apply([]byte(`{"source":"claude","status":{"state":"waiting_user"}}`), now.Add(12*time.Second))
	v = tracker.View(now.Add(12 * time.Second))
	if v.Source != "claude" || v.Content != "" || v.State != "waiting_user" {
		t.Fatal(v)
	}
	tracker.Clear()
	if tracker.View(now).Connected {
		t.Fatal("forgotten client stayed connected")
	}
}

func TestSnapshotValidationAndHiddenQueue(t *testing.T) {
	var tracker Tracker
	for _, body := range []string{`null`, `{}`, `{"source":"codex","status":null}`, `{"source":"../../bad","status":{}}`, `{"source":"codex","other":1,"status":{}}`, `{"source":"codex","sessions":{"agentId":"other","sessions":[]}}`, `{"source":"codex","sessions":{"agentId":"codex","sessions":[{"id":"a"},{"id":"a"}]}}`} {
		if tracker.Apply([]byte(body), time.Now()) == nil {
			t.Fatal("accepted", body)
		}
	}
	now := time.Now()
	err := tracker.Apply([]byte(`{"source":"codex","status":{"state":"working"},"sessions":{"agentId":"codex","displayEnabled":false,"sessions":[{"id":"a","state":"working"}]}}`), now)
	if err != nil || tracker.View(now).State != "idle" {
		t.Fatal(err, tracker.View(now))
	}
}
