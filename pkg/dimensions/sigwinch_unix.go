//go:build unix

package dimensions

import (
	"os"
	"os/signal"
	"syscall"
)

// registerSigwinch delivers SIGWINCH to ch. The Viewer owns the registration
// and releases it with signal.Stop when it stops.
func registerSigwinch(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGWINCH)
}
