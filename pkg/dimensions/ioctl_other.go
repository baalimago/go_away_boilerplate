//go:build !unix

package dimensions

import "fmt"

// DefaultReader returns a Reader that always fails with a wrapped
// ErrUnavailable. Terminal dimensions are a Unix-only feature; this stub
// keeps the package compiling on other platforms without pretending to read
// a size.
func DefaultReader(fd uintptr) Reader {
	return func() (Dimensions, error) {
		return Dimensions{}, fmt.Errorf("terminal dimensions unsupported on this platform (fd %d): %w", fd, ErrUnavailable)
	}
}
