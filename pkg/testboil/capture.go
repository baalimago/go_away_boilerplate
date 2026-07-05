package testboil

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/baalimago/go_away_boilerplate/pkg/ancli"
)

// CaptureStdout when do is called. Restore stdout as test cleanup
func CaptureStdout(t *testing.T, do func(t *testing.T)) string {
	t.Helper()
	ancli.OutMu.Lock()
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	ancli.OutMu.Unlock()

	t.Cleanup(func() {
		ancli.OutMu.Lock()
		os.Stdout = orig
		ancli.OutMu.Unlock()
	})

	do(t)

	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()
	w.Close()
	return <-outC
}

// CaptureStderr content and then restore it once the test is done
func CaptureStderr(t *testing.T, do func(t *testing.T)) string {
	t.Helper()
	ancli.ErrMut.Lock()
	orig := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	ancli.ErrMut.Unlock()

	t.Cleanup(func() {
		ancli.ErrMut.Lock()
		os.Stderr = orig
		ancli.ErrMut.Unlock()
	})

	do(t)

	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()
	w.Close()
	return <-outC
}
