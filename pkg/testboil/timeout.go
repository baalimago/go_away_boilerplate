// testboil contains functions consistently reused for testing.
// It doesn't have any dependencies except standard library and go_away_boilerplate
package testboil

import (
	"sync"
	"testing"
	"time"

	"github.com/baalimago/go_away_boilerplate/pkg/threadsafe"
)

// CheckEqualsWithinTimeout by polling at pollRate. Will at most block for timeout, when it will return false
// in case that curr != want
func CheckEqualsWithinTimeout[T comparable](currMu *sync.Mutex, curr *T, want T, timeout, pollRate time.Duration) bool {
	checkDone := time.After(timeout)
	ticker := time.NewTicker(pollRate)
	defer ticker.Stop()
	for {
		select {
		case <-checkDone:
			return false
		case <-ticker.C:
			if threadsafe.Read(currMu, curr) == want {
				return true
			}
		}
	}
}

// CheckTrueWithinTimeout polls callback at pollRate. It returns once callback returns true.
// If callback never returns true before timeout, this helper fails the test.
func CheckTrueWithinTimeout(t *testing.T, callback func(t *testing.T) bool, timeout, pollRate time.Duration) {
	t.Helper()

	checkDone := time.After(timeout)
	ticker := time.NewTicker(pollRate)
	defer ticker.Stop()
	for {
		select {
		case <-checkDone:
			t.Fatalf("callback did not return true within timeout %v", timeout)
		case <-ticker.C:
			if callback(t) {
				return
			}
		}
	}
}
