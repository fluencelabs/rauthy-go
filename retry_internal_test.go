package rauthy

import (
	"testing"
	"time"
)

// A shift that pushes the base delay's high bits out of the int64 can leave a
// small positive window. Past the cap, the window is the cap.
func TestBackoff_ShiftDoesNotOverflowBelowTheCap(t *testing.T) {
	t.Parallel()

	p := retryPolicy{maxAttempts: 64, baseDelay: 1<<40 + 1, maxDelay: 1 << 41}
	for attempt := 1; attempt <= 64; attempt++ {
		if got := p.backoff(attempt); got < p.maxDelay/2 && attempt > 1 {
			t.Fatalf("backoff(%d) = %v, want at least half the cap (%v)", attempt, got, p.maxDelay/2)
		}
	}
}

func TestBackoff_ZeroBaseDelayIsNoDelay(t *testing.T) {
	t.Parallel()

	p := retryPolicy{maxAttempts: 4, baseDelay: 0, maxDelay: time.Hour}
	for attempt := 1; attempt <= 4; attempt++ {
		if got := p.backoff(attempt); got != 0 {
			t.Errorf("backoff(%d) = %v, want 0", attempt, got)
		}
	}
}
