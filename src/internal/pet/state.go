package pet

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

type View struct {
	State, Source, Title, Content string
	SessionID                     string
	TransitionRevision            uint64
	Connected                     bool
}
type status struct {
	State          string `json:"state"`
	Status         string `json:"status"`
	Event          string `json:"event"`
	StatusText     string `json:"statusText"`
	SessionID      string `json:"sessionId"`
	SessionTitle   string `json:"sessionTitle"`
	DisplayTitle   string `json:"displayTitle"`
	DisplayContent string `json:"displayContent"`
	Text           string `json:"text"`
	Message        string `json:"message"`
	Title          string `json:"title"`
}
type session struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	State     string `json:"state"`
	Revision  uint64 `json:"transitionRevision"`
	Remaining int64  `json:"terminalRemainingMs"`
	deadline  time.Time
}
type sessionQueue struct {
	Agent    string    `json:"agentId"`
	Selected string    `json:"sessionId"`
	Enabled  bool      `json:"displayEnabled"`
	Sessions []session `json:"sessions"`
}
type snapshot struct {
	Source   string        `json:"source"`
	Status   *status       `json:"status"`
	Speech   *status       `json:"speech"`
	Sessions *sessionQueue `json:"sessions"`
}

// Tracker is a bounded, read-only presentation sink. Probes keep connectivity
// alive, but never extend a snapshot's lifetime or a terminal session deadline.
type Tracker struct {
	mu             sync.Mutex
	current        snapshot
	seen, received time.Time
}

func cleanText(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	if len(s) > limit {
		s = s[:limit]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

func motion(s string) string {
	switch strings.ToLower(s) {
	case "working", "thinking", "tool_running", "speaking", "running", "executing", "responding", "busy", "typing", "browsing":
		return "working"
	case "waiting_user", "notification", "waiting", "permission", "approval_required", "decide":
		return "waiting_user"
	case "done", "completed", "complete", "success", "reply_complete":
		return "done"
	case "error", "failed", "failure":
		return "error"
	default:
		return "idle"
	}
}
func terminal(s string) bool { m := motion(s); return m == "done" || m == "error" }
func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (t *Tracker) Probe(now time.Time) { t.mu.Lock(); t.seen = now; t.mu.Unlock() }
func (t *Tracker) Clear() {
	t.mu.Lock()
	t.current = snapshot{}
	t.seen = time.Time{}
	t.received = time.Time{}
	t.mu.Unlock()
}

func (t *Tracker) Apply(data []byte, now time.Time) error {
	if len(data) == 0 || len(data) > 16384 || !utf8.Valid(data) {
		return errors.New("invalid LAN snapshot")
	}
	var envelope map[string]json.RawMessage
	var next snapshot
	if json.Unmarshal(data, &envelope) != nil || json.Unmarshal(data, &next) != nil || next.Source == "" || len(next.Source) > 63 {
		return errors.New("invalid LAN snapshot")
	}
	for _, c := range next.Source {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return errors.New("invalid LAN source")
		}
	}
	for k, v := range envelope {
		if k != "source" && k != "status" && k != "speech" && k != "sessions" || string(v) == "null" {
			return errors.New("invalid LAN envelope")
		}
	}
	if next.Status == nil && next.Speech == nil && next.Sessions == nil {
		return errors.New("empty LAN snapshot")
	}
	if q := next.Sessions; q != nil {
		if q.Agent != next.Source || len(q.Sessions) > 8 {
			return errors.New("invalid LAN sessions")
		}
		ids := map[string]bool{}
		for _, row := range q.Sessions {
			if row.ID == "" || len(row.ID) > 127 || ids[row.ID] || row.Remaining < 0 || row.Remaining > 60000 || row.Revision > 9007199254740991 {
				return errors.New("invalid LAN session")
			}
			ids[row.ID] = true
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if q := next.Sessions; q != nil {
		old := map[string]session{}
		if t.current.Source == next.Source && t.current.Sessions != nil {
			for _, row := range t.current.Sessions.Sessions {
				old[row.ID] = row
			}
		}
		for i, row := range q.Sessions {
			previous, exists := old[row.ID]
			if exists && row.Revision < previous.Revision {
				q.Sessions[i] = previous
				continue
			}
			if terminal(row.State) {
				row.deadline = now.Add(time.Duration(row.Remaining) * time.Millisecond)
				if exists && row.Revision == previous.Revision && terminal(previous.State) && previous.deadline.Before(row.deadline) {
					row.deadline = previous.deadline
				}
			}
			q.Sessions[i] = row
		}
	}
	t.current, t.received, t.seen = next, now, now
	return nil
}

func (t *Tracker) View(now time.Time) View {
	t.mu.Lock()
	defer t.mu.Unlock()
	v := View{State: "idle", Connected: !t.seen.IsZero() && now.Sub(t.seen) < 10*time.Second}
	if t.received.IsZero() || now.Sub(t.received) >= 10*time.Second {
		return v
	}
	s := t.current
	v.Source = s.Source
	if q := s.Sessions; q != nil {
		// A session snapshot is authoritative, including an empty/hidden queue.
		// Aggregate working state cannot override a terminal selected session.
		if !q.Enabled {
			return v
		}
		var rows []session
		for _, row := range q.Sessions {
			if motion(row.State) == "idle" || terminal(row.State) && !now.Before(row.deadline) {
				continue
			}
			rows = append(rows, row)
		}
		if len(rows) == 0 {
			return v
		}
		selected := rows[0]
		for _, row := range rows {
			if row.ID == q.Selected {
				selected = row
				break
			}
		}
		v.State, v.Title, v.Content = motion(selected.State), cleanText(selected.Title, 191), cleanText(selected.Content, 383)
		v.SessionID, v.TransitionRevision = selected.ID, selected.Revision
		if s.Speech != nil && s.Speech.SessionID == selected.ID {
			v.Content = cleanText(first(s.Speech.Text, s.Speech.Message, s.Speech.DisplayContent), 383)
		}
		return v
	}
	if st := s.Status; st != nil {
		v.SessionID = st.SessionID
		v.State = motion(first(st.Status, st.State, st.Event))
		v.Title = cleanText(first(st.DisplayTitle, st.SessionTitle, st.Title), 191)
		v.Content = cleanText(first(st.DisplayContent, st.Text, st.Message, st.StatusText), 383)
	}
	if st := s.Speech; st != nil {
		v.Title = cleanText(first(st.Title, v.Title), 191)
		v.Content = cleanText(first(st.Text, st.Message, st.DisplayContent, v.Content), 383)
	}
	return v
}
