// Package debounce provides a timer that delays calling a function until
// a quiet period has elapsed with no new triggers (FR-01d, FR-07b).
package debounce

import (
	"sync"
	"time"
)

// Timer calls fn at most once per delay period, restarting the countdown
// on each Trigger call. Safe for concurrent use.
type Timer struct {
	delay  time.Duration
	fn     func()
	mu     sync.Mutex
	timer  *time.Timer
	stopped bool
}

// New creates a Timer that will call fn after delay has elapsed since the
// last Trigger call.
func New(delay time.Duration, fn func()) *Timer {
	return &Timer{delay: delay, fn: fn}
}

// Trigger resets the countdown. If the timer is already pending it is
// restarted. A no-op after Stop.
func (t *Timer) Trigger() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return
	}
	if t.timer != nil {
		t.timer.Stop()
	}
	t.timer = time.AfterFunc(t.delay, t.fn)
}

// Stop cancels any pending call. Safe to call multiple times.
func (t *Timer) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopped = true
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
}
