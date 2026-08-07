//go:build linux

package dimensions

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"unsafe"
)

// openPtyMaster opens a fresh pseudo-terminal master. On Linux the master
// accepts TIOCGWINSZ and TIOCSWINSZ directly, so the tests need no slave.
func openPtyMaster(t *testing.T) *os.File {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open /dev/ptmx: %v", err)
	}
	t.Cleanup(func() { master.Close() })
	return master
}

func setWinsize(t *testing.T, fd uintptr, ws *winsize) {
	t.Helper()
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(ws)),
	)
	if errno != 0 {
		t.Fatalf("TIOCSWINSZ: %v", errno)
	}
}

// TestDefaultReader_ReadsPtySize verifies one live TIOCGWINSZ query returns
// width and height together from a real terminal.
func TestDefaultReader_ReadsPtySize(t *testing.T) {
	master := openPtyMaster(t)
	setWinsize(t, master.Fd(), &winsize{Row: 24, Col: 80})

	d, err := DefaultReader(master.Fd())()
	if err != nil {
		t.Fatalf("DefaultReader() error: %v", err)
	}
	if d != (Dimensions{Width: 80, Height: 24}) {
		t.Fatalf("DefaultReader() = %+v, want {80 24}", d)
	}
}

// TestDefaultReader_ZeroSize verifies that a terminal reporting zero width or
// height is treated as unavailable rather than fabricating a size.
func TestDefaultReader_ZeroSize(t *testing.T) {
	master := openPtyMaster(t)
	setWinsize(t, master.Fd(), &winsize{}) // explicit 0x0

	d, err := DefaultReader(master.Fd())()
	if err == nil || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("DefaultReader() error = %v, want wrapped ErrUnavailable", err)
	}
	if d != (Dimensions{}) {
		t.Fatalf("DefaultReader() = %+v, want zero Dimensions on failure", d)
	}
}
