package dimensions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// readResult is one deterministic answer for an injected Reader.
type readResult struct {
	d   Dimensions
	err error
}

// chanResultReader returns a Reader that consumes one readResult per call.
// Each call blocks until the test supplies the next result, so test ordering
// is deterministic.
func chanResultReader(results chan readResult) Reader {
	return func() (Dimensions, error) {
		r := <-results
		return r.d, r.err
	}
}

func errUnavailable(msg string) error {
	return fmt.Errorf("%s: %w", msg, ErrUnavailable)
}

func newTestViewer(t *testing.T, results chan readResult) (*Viewer, chan os.Signal) {
	t.Helper()
	sigSrc := make(chan os.Signal, 64)
	v := New(context.Background(), 0, WithReader(chanResultReader(results)), WithSignalSource(sigSrc))
	t.Cleanup(v.Stop)
	return v, sigSrc
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("condition not met within %v", timeout)
}

func requireEventsClosed(t *testing.T, v *Viewer) {
	t.Helper()
	select {
	case d, ok := <-v.Events():
		if ok {
			t.Fatalf("events channel still open, got %v", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("events channel was not closed")
	}
}

func requireNoEvent(t *testing.T, v *Viewer, wait time.Duration) {
	t.Helper()
	select {
	case d, ok := <-v.Events():
		t.Fatalf("unexpected event %v (open=%v)", d, ok)
	case <-time.After(wait):
	}
}

func TestSnapshot_LiveRead(t *testing.T) {
	results := make(chan readResult, 2)
	results <- readResult{d: Dimensions{Width: 120, Height: 40}}
	results <- readResult{d: Dimensions{Width: 100, Height: 30}}
	v, _ := newTestViewer(t, results)

	d, err := v.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}
	if d != (Dimensions{Width: 100, Height: 30}) {
		t.Fatalf("Snapshot() = %+v, want {100 30}", d)
	}
}

func TestSnapshot_InitialReadFailure_ReturnsFallback(t *testing.T) {
	v := New(context.Background(), 0,
		WithReader(func() (Dimensions, error) {
			return Dimensions{}, errUnavailable("boom")
		}),
		WithSignalSource(make(chan os.Signal)),
	)
	defer v.Stop()

	d, err := v.Snapshot()
	if err == nil {
		t.Fatal("Snapshot() error = nil, want wrapped ErrUnavailable")
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Snapshot() error %v does not wrap ErrUnavailable", err)
	}
	if d != Fallback {
		t.Fatalf("Snapshot() = %+v, want fallback %+v", d, Fallback)
	}
}

func TestSnapshot_ZeroSizeIsUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name string
		dims Dimensions
	}{
		{name: "zero width", dims: Dimensions{Width: 0, Height: 24}},
		{name: "zero height", dims: Dimensions{Width: 80, Height: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := New(context.Background(), 0,
				WithReader(func() (Dimensions, error) {
					return tc.dims, nil
				}),
				WithSignalSource(make(chan os.Signal)),
			)
			defer v.Stop()

			d, err := v.Snapshot()
			if err == nil || !errors.Is(err, ErrUnavailable) {
				t.Fatalf("Snapshot() error = %v, want wrapped ErrUnavailable", err)
			}
			if d != Fallback {
				t.Fatalf("Snapshot() = %+v, want fallback %+v", d, Fallback)
			}
		})
	}
}

func TestSnapshot_RefreshFailsAfterValid_KeepsLastValid(t *testing.T) {
	results := make(chan readResult, 4)
	results <- readResult{d: Dimensions{Width: 100, Height: 30}} // initial read
	v, sigSrc := newTestViewer(t, results)

	// Live read fails after a valid snapshot: last valid size is preserved
	// together with the wrapped error.
	results <- readResult{err: errUnavailable("ioctl failed")}
	d, err := v.Snapshot()
	if err == nil || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Snapshot() error = %v, want wrapped ErrUnavailable", err)
	}
	if d != (Dimensions{Width: 100, Height: 30}) {
		t.Fatalf("Snapshot() = %+v, want last valid {100 30}", d)
	}

	// A signal-driven refresh that fails delivers no event.
	sigSrc <- syscall.SIGWINCH
	results <- readResult{err: errUnavailable("ioctl failed again")}
	waitFor(t, 2*time.Second, func() bool { return len(results) == 0 })
	requireNoEvent(t, v, 50*time.Millisecond)

	// The reader recovers: Snapshot returns the fresh size.
	results <- readResult{d: Dimensions{Width: 90, Height: 50}}
	d, err = v.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error after recovery: %v", err)
	}
	if d != (Dimensions{Width: 90, Height: 50}) {
		t.Fatalf("Snapshot() = %+v, want {90 50}", d)
	}
}

func TestEvents_DeliversFreshSnapshotAfterSignal(t *testing.T) {
	results := make(chan readResult, 4)
	results <- readResult{d: Dimensions{Width: 90, Height: 50}} // initial read
	v, sigSrc := newTestViewer(t, results)

	sigSrc <- syscall.SIGWINCH
	results <- readResult{d: Dimensions{Width: 91, Height: 51}}
	select {
	case d := <-v.Events():
		if d != (Dimensions{Width: 91, Height: 51}) {
			t.Fatalf("event = %+v, want {91 51}", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event after signal")
	}

	sigSrc <- syscall.SIGWINCH
	results <- readResult{d: Dimensions{Width: 92, Height: 52}}
	select {
	case d := <-v.Events():
		if d != (Dimensions{Width: 92, Height: 52}) {
			t.Fatalf("event = %+v, want {92 52}", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event after second signal")
	}
}

func TestEvents_CoalescesBurst(t *testing.T) {
	const burst = 1000
	var calls atomic.Int32
	sigSrc := make(chan os.Signal, burst)
	v := New(context.Background(), 0,
		WithReader(func() (Dimensions, error) {
			calls.Add(1)
			return Dimensions{Width: 80, Height: 24}, nil
		}),
		WithSignalSource(sigSrc),
	)
	defer v.Stop()

	for range burst {
		sigSrc <- syscall.SIGWINCH
	}
	// Wait until the watcher processed every signal: it must never block on
	// the full event channel. The counter ends at exactly initial + burst
	// reads, because each of the burst signals triggers exactly one refresh.
	waitFor(t, 5*time.Second, func() bool { return calls.Load() == burst+1 })

	select {
	case d, ok := <-v.Events():
		if !ok {
			t.Fatal("events channel closed while watching")
		}
		if d != (Dimensions{Width: 80, Height: 24}) {
			t.Fatalf("event = %+v, want {80 24}", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected one buffered event")
	}
	// Only one event survived the burst: the rest were coalesced.
	select {
	case d, ok := <-v.Events():
		t.Fatalf("second event %v (open=%v), want coalesced", d, ok)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestStop_IsIdempotent_AndClosesEvents(t *testing.T) {
	results := make(chan readResult, 1)
	results <- readResult{d: Dimensions{Width: 120, Height: 40}}
	v, _ := newTestViewer(t, results)

	v.Stop()
	requireEventsClosed(t, v)

	// Second stop returns immediately and cleanup stays idempotent.
	done := make(chan struct{})
	go func() {
		v.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("second Stop() blocked")
	}
}

func TestSnapshot_WorksAfterStop(t *testing.T) {
	results := make(chan readResult, 2)
	results <- readResult{d: Dimensions{Width: 120, Height: 40}}
	results <- readResult{d: Dimensions{Width: 110, Height: 35}}
	v, _ := newTestViewer(t, results)

	v.Stop()
	d, err := v.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error after stop: %v", err)
	}
	if d != (Dimensions{Width: 110, Height: 35}) {
		t.Fatalf("Snapshot() = %+v, want {110 35}", d)
	}
}

func TestContextCancellation_StopsWatcher(t *testing.T) {
	results := make(chan readResult, 1)
	results <- readResult{d: Dimensions{Width: 80, Height: 24}}
	ctx, cancel := context.WithCancel(context.Background())
	v := New(ctx, 0, WithReader(chanResultReader(results)), WithSignalSource(make(chan os.Signal)))

	cancel()
	requireEventsClosed(t, v)

	// Stop after context cancellation returns immediately.
	done := make(chan struct{})
	go func() {
		v.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() blocked after context cancellation")
	}
}

func TestSignalSourceClosed_StopsWatcher(t *testing.T) {
	results := make(chan readResult, 1)
	results <- readResult{d: Dimensions{Width: 80, Height: 24}}
	sigSrc := make(chan os.Signal)
	v := New(context.Background(), 0, WithReader(chanResultReader(results)), WithSignalSource(sigSrc))

	close(sigSrc)
	requireEventsClosed(t, v)

	done := make(chan struct{})
	go func() {
		v.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() blocked after signal source closed")
	}
}

func TestNew_DefaultSignalRegistration(t *testing.T) {
	// No WithSignalSource: the Viewer registers SIGWINCH process-wide and
	// must release the registration on Stop. The reader is injected so the
	// test is deterministic.
	v := New(context.Background(), 0, WithReader(func() (Dimensions, error) {
		return Dimensions{Width: 100, Height: 30}, nil
	}))
	d, err := v.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}
	if d != (Dimensions{Width: 100, Height: 30}) {
		t.Fatalf("Snapshot() = %+v, want {100 30}", d)
	}
	v.Stop()
	requireEventsClosed(t, v)
}

func TestNew_NilReader_UsesDefaultReader(t *testing.T) {
	v := New(context.Background(), 0, WithReader(nil), WithSignalSource(make(chan os.Signal)))
	defer v.Stop()

	d, err := v.Snapshot()
	if err == nil {
		if d.Width <= 0 || d.Height <= 0 {
			t.Fatalf("Snapshot() = %+v, want positive dimensions", d)
		}
		return
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Snapshot() error = %v, want wrapped ErrUnavailable", err)
	}
}

// runInSubprocess re-executes the current test binary with the given test
// name and an environment variable that selects the child branch. Returns the
// exit code and any non-ExitError error.
func runInSubprocess(t *testing.T, testName, envVar string) (int, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run="+testName)
	cmd.Env = append(os.Environ(), envVar+"=1")
	if err := cmd.Run(); err == nil {
		return 0, nil
	} else if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), nil
	} else {
		return -1, err
	}
}

// TestViewer_RealSIGWINCH exercises the process-wide signal path end to end.
// It runs in a subprocess because SIGWINCH is process-global: the child sends
// the signal to itself and must not contaminate other tests.
func TestViewer_RealSIGWINCH(t *testing.T) {
	if os.Getenv("TEST_DIM_REAL_SIGWINCH") == "1" {
		v := New(context.Background(), 0, WithReader(func() (Dimensions, error) {
			return Dimensions{Width: 123, Height: 45}, nil
		}))
		syscall.Kill(syscall.Getpid(), syscall.SIGWINCH)
		select {
		case d := <-v.Events():
			if d != (Dimensions{Width: 123, Height: 45}) {
				os.Exit(2)
			}
			os.Exit(0)
		case <-time.After(3 * time.Second):
			os.Exit(3)
		}
	}
	code, err := runInSubprocess(t, "TestViewer_RealSIGWINCH", "TEST_DIM_REAL_SIGWINCH")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}
