// Package debounce implements a debounce timer for rate-limiting rapid calls.
// Tests cover FR-01d (80 ms AI debounce) and FR-07 (timing requirements).
package debounce_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/dinav2/vida/internal/debounce"
)

// FR-01d: function not called until delay has elapsed with no reset.
func TestDebounce_FiresAfterDelay(t *testing.T) {
	var count atomic.Int32
	d := debounce.New(50*time.Millisecond, func() { count.Add(1) })
	defer d.Stop()

	d.Trigger()
	time.Sleep(20 * time.Millisecond)
	if count.Load() != 0 {
		t.Error("fn called before delay elapsed")
	}
	time.Sleep(60 * time.Millisecond)
	if count.Load() != 1 {
		t.Errorf("fn call count = %d, want 1", count.Load())
	}
}

// FR-01d: rapid triggers coalesce into one call.
func TestDebounce_RapidTriggersCoalesce(t *testing.T) {
	var count atomic.Int32
	d := debounce.New(60*time.Millisecond, func() { count.Add(1) })
	defer d.Stop()

	for i := 0; i < 10; i++ {
		d.Trigger()
		time.Sleep(10 * time.Millisecond)
	}
	// Wait for debounce to fire.
	time.Sleep(100 * time.Millisecond)
	if count.Load() != 1 {
		t.Errorf("fn call count = %d, want 1 (coalesced)", count.Load())
	}
}

// FR-01d: resetting restarts the timer.
func TestDebounce_ResetRestartsTimer(t *testing.T) {
	var count atomic.Int32
	d := debounce.New(60*time.Millisecond, func() { count.Add(1) })
	defer d.Stop()

	d.Trigger()
	time.Sleep(40 * time.Millisecond)
	d.Trigger() // reset
	time.Sleep(40 * time.Millisecond)
	// 80ms total elapsed since first trigger but only 40ms since reset.
	if count.Load() != 0 {
		t.Error("fn fired before reset delay elapsed")
	}
	time.Sleep(40 * time.Millisecond)
	if count.Load() != 1 {
		t.Errorf("fn call count = %d, want 1 after reset delay", count.Load())
	}
}

// Stop prevents any pending fire.
func TestDebounce_StopCancelsPending(t *testing.T) {
	var count atomic.Int32
	d := debounce.New(50*time.Millisecond, func() { count.Add(1) })

	d.Trigger()
	d.Stop()
	time.Sleep(100 * time.Millisecond)
	if count.Load() != 0 {
		t.Error("fn called after Stop()")
	}
}

// Trigger after Stop is a no-op (no panic, no fire).
func TestDebounce_TriggerAfterStopNoop(t *testing.T) {
	d := debounce.New(20*time.Millisecond, func() {})
	d.Stop()
	// Should not panic.
	d.Trigger()
	time.Sleep(40 * time.Millisecond)
}
