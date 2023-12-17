// testboil contains functions consistently reused for testing.
// It doesn't have any dependencies except standard library and go_away_boilerplate
package testboil

import (
	"sync"
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
