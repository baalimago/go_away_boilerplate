package testboil

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/baalimago/go_away_boilerplate/pkg/threadsafe"
)

// ReturnsOnContextCancel by creating a context and ensuring that the function returns
// once the context has been cancelled
func ReturnsOnContextCancel(t *testing.T, f func(context.Context), testTimeout time.Duration) {
	t.Run("it should break on context cancel", func(t *testing.T) {
		if !testPass(f, testTimeout) {
			t.Log("function failed to return within timeout")
			t.Fail()
		}
	})
}

func testPass(f func(context.Context), testTimeout time.Duration) bool {
	testCtx, testCtxCancel := context.WithTimeout(context.Background(), testTimeout)
	isDone := false
	isDoneMu := &sync.Mutex{}
	hasStarted := make(chan struct{})
	go func() {
		close(hasStarted)
		f(testCtx)
		threadsafe.Write(isDoneMu, true, &isDone)
	}()
	<-hasStarted
	testCtxCancel()
	// Give 100 iterations to check if it has managed to quit
	if CheckEqualsWithinTimeout(isDoneMu, &isDone, true, testTimeout, testTimeout/100) {
		return true
	}
	return false
}
