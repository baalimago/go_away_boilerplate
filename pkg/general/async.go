package general

import (
	"sync"
	"time"
)

// RaceSafeWrite by locking the mutext before writing
func RaceSafeWrite[T any](m *sync.Mutex, value T, dest *T) {
	m.Lock()
	defer m.Unlock()
	*dest = value
}

// RaceSafeRead by locking the mutex before taking a copy, will then return the copy
func RaceSafeRead[T any](m *sync.Mutex, src *T) T {
	m.Lock()
	defer m.Unlock()
	return *src
}

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
			if RaceSafeRead(currMu, curr) == want {
				return true
			}
		}
	}
}
