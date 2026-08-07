//go:build unix

package dimensions

import (
	"fmt"
	"syscall"
	"unsafe"
)

// winsize mirrors the C struct winsize used by TIOCGWINSZ: row, column, and
// the pixel sizes in that order.
type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

// DefaultReader returns a Reader that queries fd with one TIOCGWINSZ ioctl
// and returns width and height together. A failed ioctl or a reported zero
// width or height yields zero Dimensions and a wrapped ErrUnavailable; the
// reader never fabricates a size. Non-terminal descriptors fail the ioctl
// with ENOTTY and are ordinary ErrUnavailable failures.
func DefaultReader(fd uintptr) Reader {
	return func() (Dimensions, error) {
		ws := &winsize{}
		_, _, errno := syscall.Syscall(
			syscall.SYS_IOCTL,
			fd,
			uintptr(syscall.TIOCGWINSZ),
			uintptr(unsafe.Pointer(ws)),
		)
		if errno != 0 {
			return Dimensions{}, fmt.Errorf("ioctl TIOCGWINSZ on fd %d: %w", fd, ErrUnavailable)
		}
		if ws.Col == 0 || ws.Row == 0 {
			return Dimensions{}, fmt.Errorf("terminal on fd %d reports zero size: %w", fd, ErrUnavailable)
		}
		return Dimensions{Width: int(ws.Col), Height: int(ws.Row)}, nil
	}
}
