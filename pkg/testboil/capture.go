package testboil

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// CaptureStdout when do is called. Restore stdout as test cleanup
func CaptureStdout(t *testing.T, do func(t *testing.T)) string {
	t.Helper()
	orig := os.Stdout
	t.Cleanup(func() {
		os.Stdout = orig
	})

	r, w, _ := os.Pipe()
	os.Stdout = w
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
	orig := os.Stderr
	t.Cleanup(func() {
		os.Stderr = orig
	})

	r, w, _ := os.Pipe()
	os.Stderr = w
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
