//go:build !unix

package dimensions

import "os"

// registerSigwinch is a no-op on platforms without SIGWINCH. Terminal
// dimensions are a Unix-only feature; on other platforms the Viewer only
// observes injected signal sources and Snapshot.
func registerSigwinch(ch chan<- os.Signal) {}
