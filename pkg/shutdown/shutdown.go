package shutdown

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/baalimago/go_away_boilerplate/pkg/ancli"
)

// Monitor listens for a shutdown signal and cancels the context
// if the signal is received. If the signal is received again, it will
// force a shutdown.
func Monitor(cancel context.CancelFunc) {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	amountOfCancels := 0
	for {
		<-signalCh
		switch amountOfCancels {
		case 0:
			ancli.PrintWarn("initiated forceful shutdown\n")
			cancel()
		case 1:
			ancli.PrintWarn(
				"graceful shutdown ongoing, cancel again to force shutdown\n",
			)
		default:
			ancli.PrintErr("forcing shutdown\n")
			os.Exit(1)
		}
		amountOfCancels++
	}
}

// MonitorV2 is the same as Monitor except for two points:
// 1. It breaks on ctx cancel
// 2. It doesn't append newline on the warns or error prints
//
// The first signal cancels; since cancelling closes ctx.Done, the escalation
// ladder must survive MonitorV2's return: signal.Notify suppresses default
// SIGINT/SIGTERM handling process-wide, so returning while still notified
// would make every later Ctrl-C a silent no-op and leave a stuck graceful
// shutdown unkillable short of SIGKILL. A signal-initiated return therefore
// hands signalCh to a detached drainer that keeps escalating (warn, then
// force-exit), while a return on normal completion stops notification so the
// OS default handling applies again.
func MonitorV2(ctx context.Context, cancel context.CancelFunc) {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	amountOfCancels := 0
	for {
		select {
		case <-ctx.Done():
			if amountOfCancels == 0 {
				signal.Stop(signalCh)
				return
			}
			go drainSignals(signalCh, amountOfCancels)
			return
		case <-signalCh:
			switch amountOfCancels {
			case 0:
				ancli.PrintWarn("initiated forceful shutdown")
				cancel()
			case 1:
				ancli.PrintWarn(
					"graceful shutdown ongoing, cancel again to force shutdown",
				)
			default:
				ancli.PrintErr("forcing shutdown")
				os.Exit(1)
			}
			amountOfCancels++
		}
	}
}

// drainSignals continues MonitorV2's escalation ladder after it has returned
// on a signal-initiated cancel. It runs until process exit: either the
// graceful shutdown completes and the process dies with this goroutine, or
// the escalation reaches force-exit.
func drainSignals(signalCh chan os.Signal, amountOfCancels int) {
	for {
		<-signalCh
		if amountOfCancels == 1 {
			ancli.PrintWarn(
				"graceful shutdown ongoing, cancel again to force shutdown",
			)
		} else {
			ancli.PrintErr("forcing shutdown")
			os.Exit(1)
		}
		amountOfCancels++
	}
}
