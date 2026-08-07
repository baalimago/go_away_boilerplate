package dimensions

import (
	"errors"
	"os"
	"testing"
)

// TestDefaultReader_NonTerminalFd verifies that a descriptor without a
// terminal size (here: a pipe) is an ordinary ErrUnavailable failure and that
// the reader never fabricates a size.
func TestDefaultReader_NonTerminalFd(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	d, err := DefaultReader(w.Fd())()
	if err == nil {
		t.Fatal("DefaultReader() error = nil, want wrapped ErrUnavailable")
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("DefaultReader() error = %v, want wrapped ErrUnavailable", err)
	}
	if d != (Dimensions{}) {
		t.Fatalf("DefaultReader() = %+v, want zero Dimensions on failure", d)
	}
}
