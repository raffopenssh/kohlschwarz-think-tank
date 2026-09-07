package jobs

import (
	"sync"
	"time"
)

// Activity tracks the one background job (fetch / rank / email) that may run at a time.
type Activity struct {
	mu      sync.Mutex
	kind    string
	started time.Time
	done    time.Time
	last    string // summary of last finished job
}

var Current = &Activity{}

// Start claims the activity slot; returns false if something is already running.
func (a *Activity) Start(kind string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.kind != "" {
		return false
	}
	a.kind, a.started = kind, time.Now()
	return true
}

// Switch relabels the running activity (e.g. fetch → rank) without releasing the slot.
func (a *Activity) Switch(kind string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.kind != "" {
		a.kind, a.started = kind, time.Now()
	}
}

// Finish releases the slot and records a summary line.
func (a *Activity) Finish(summary string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.kind, a.done, a.last = "", time.Now(), summary
}

type ActivityState struct {
	Running string `json:"running"`         // "" | fetch | rank | brief | email
	Since   int64  `json:"since,omitempty"` // unix seconds
	Done    int64  `json:"done,omitempty"`  // unix seconds of last completion
	Last    string `json:"last,omitempty"`
}

func (a *Activity) State() ActivityState {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := ActivityState{Running: a.kind, Last: a.last}
	if a.kind != "" {
		s.Since = a.started.Unix()
	}
	if !a.done.IsZero() {
		s.Done = a.done.Unix()
	}
	return s
}

// Ago renders a UTC "YYYY-MM-DD HH:MM:SS" timestamp as a short relative string.
func Ago(ts string) string {
	t, err := time.Parse("2006-01-02 15:04:05", ts)
	if err != nil {
		return ts
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return itoa(int(d.Minutes())) + " min ago"
	case d < 48*time.Hour:
		return itoa(int(d.Hours())) + " h ago"
	default:
		return itoa(int(d.Hours()/24)) + " d ago"
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}
