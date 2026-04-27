package testboil

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckTrueWithinTimeout(t *testing.T) {
	t.Run("returns when callback becomes true before timeout", func(t *testing.T) {
		var calls atomic.Int32
		go func() {
			time.Sleep(10 * time.Millisecond)
			calls.Store(1)
		}()

		CheckTrueWithinTimeout(t, func() bool {
			return calls.Load() == 1
		}, 100*time.Millisecond, 5*time.Millisecond)
	})

	t.Run("fails the test when callback never becomes true", func(t *testing.T) {
		passed := !testPass(func(ctx context.Context) {
			_ = ctx
			mockT := &testing.T{}
			CheckTrueWithinTimeout(mockT, func() bool {
				return false
			}, 20*time.Millisecond, 5*time.Millisecond)
		}, 100*time.Millisecond)

		if !passed {
			t.Fatal("expected test to fail when callback stays false")
		}
	})
}
