package shutdown

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Subprocess helpers
// ---------------------------------------------------------------------------

// runInSubprocess re-executes the current test binary with the given test
// name and an environment variable set to select the child branch.
// Returns the exit code and any non-ExitError error.
func runInSubprocess(t *testing.T, testName, envVar string) (exitCode int, err error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run="+testName)
	cmd.Env = append(os.Environ(), envVar+"=1")
	if e := cmd.Run(); e == nil {
		return 0, nil
	} else if exitErr, ok := e.(*exec.ExitError); ok {
		return exitErr.ExitCode(), nil
	} else {
		return -1, e
	}
}

// ---------------------------------------------------------------------------
// Monitor – signal logic tests (all via subprocess to avoid goroutine leaks
// and cross-test signal contamination)
// ---------------------------------------------------------------------------

func TestMonitor_FirstSignalCancels(t *testing.T) {
	if os.Getenv("TEST_MON_FIRST") == "1" {
		var mu sync.Mutex
		count := 0
		cancel := func() {
			mu.Lock()
			count++
			mu.Unlock()
		}
		go Monitor(cancel)
		time.Sleep(100 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		time.Sleep(200 * time.Millisecond)
		mu.Lock()
		c := count
		mu.Unlock()
		if c != 1 {
			os.Exit(2) // failure exit code distinct from os.Exit(1)
		}
		os.Exit(0)
	}
	code, err := runInSubprocess(t, "TestMonitor_FirstSignalCancels", "TEST_MON_FIRST")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}

func TestMonitor_SecondSignalDoesNotRecancel(t *testing.T) {
	if os.Getenv("TEST_MON_SECOND") == "1" {
		var mu sync.Mutex
		count := 0
		cancel := func() {
			mu.Lock()
			count++
			mu.Unlock()
		}
		go Monitor(cancel)
		time.Sleep(100 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		time.Sleep(100 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		time.Sleep(200 * time.Millisecond)
		mu.Lock()
		c := count
		mu.Unlock()
		if c != 1 {
			os.Exit(2)
		}
		os.Exit(0)
	}
	code, err := runInSubprocess(t, "TestMonitor_SecondSignalDoesNotRecancel", "TEST_MON_SECOND")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}

func TestMonitor_ThirdSignalExits(t *testing.T) {
	if os.Getenv("TEST_MON_THIRD") == "1" {
		cancel := func() {}
		go Monitor(cancel)
		time.Sleep(100 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT) // 1st: cancel
		time.Sleep(50 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT) // 2nd: warn
		time.Sleep(50 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT) // 3rd: os.Exit(1)
		time.Sleep(500 * time.Millisecond)
		os.Exit(0) // should never reach
	}
	code, err := runInSubprocess(t, "TestMonitor_ThirdSignalExits", "TEST_MON_THIRD")
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("expected exit 1 (os.Exit), got %d", code)
	}
}

// ---------------------------------------------------------------------------
// MonitorV2 – ctx.Done path can be tested in-process (it returns cleanly).
// Signal paths tested in subprocesses.
// ---------------------------------------------------------------------------

func TestMonitorV2_CtxDoneReturns(t *testing.T) {
	// This test does NOT send signals, so it's safe to run in-process.
	cancel := func() {}
	ctx, ctxCancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		MonitorV2(ctx, cancel)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	ctxCancel()

	select {
	case <-done:
		// pass
	case <-time.After(2 * time.Second):
		t.Fatal("MonitorV2 did not return after context cancellation")
	}
}

func TestMonitorV2_FirstSignalCancels(t *testing.T) {
	if os.Getenv("TEST_MV2_FIRST") == "1" {
		var mu sync.Mutex
		count := 0
		cancel := func() {
			mu.Lock()
			count++
			mu.Unlock()
		}
		ctx, ctxCancel := context.WithCancel(context.Background())
		defer ctxCancel()
		go MonitorV2(ctx, cancel)
		time.Sleep(100 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		time.Sleep(200 * time.Millisecond)
		mu.Lock()
		c := count
		mu.Unlock()
		if c != 1 {
			os.Exit(2)
		}
		os.Exit(0)
	}
	code, err := runInSubprocess(t, "TestMonitorV2_FirstSignalCancels", "TEST_MV2_FIRST")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}

func TestMonitorV2_SecondSignalDoesNotRecancel(t *testing.T) {
	if os.Getenv("TEST_MV2_SECOND") == "1" {
		var mu sync.Mutex
		count := 0
		cancel := func() {
			mu.Lock()
			count++
			mu.Unlock()
		}
		ctx, _ := context.WithCancel(context.Background())
		go MonitorV2(ctx, cancel)
		time.Sleep(100 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		time.Sleep(100 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		time.Sleep(200 * time.Millisecond)
		mu.Lock()
		c := count
		mu.Unlock()
		if c != 1 {
			os.Exit(2)
		}
		os.Exit(0)
	}
	code, err := runInSubprocess(t, "TestMonitorV2_SecondSignalDoesNotRecancel", "TEST_MV2_SECOND")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}

func TestMonitorV2_ThirdSignalExits(t *testing.T) {
	if os.Getenv("TEST_MV2_THIRD") == "1" {
		ctx := context.Background()
		cancel := func() {}
		go MonitorV2(ctx, cancel)
		time.Sleep(100 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		time.Sleep(50 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		time.Sleep(50 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}
	code, err := runInSubprocess(t, "TestMonitorV2_ThirdSignalExits", "TEST_MV2_THIRD")
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("expected exit 1 (os.Exit), got %d", code)
	}
}
