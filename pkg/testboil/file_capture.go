package testboil

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// CaptureFile content and then restore it once the test is done
func CaptureFile(t *testing.T, f *os.File, do func(t *testing.T)) string {
	t.Helper()
	orig := f
	t.Cleanup(func() {
		f = orig
	})

	r, w, _ := os.Pipe()
	f = w
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
