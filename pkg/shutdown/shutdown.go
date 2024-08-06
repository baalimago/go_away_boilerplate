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
		select {
		case <-signalCh:
			if amountOfCancels == 0 {
				ancli.PrintWarn("initiated forceful shutdown\n")
				cancel()
			} else if amountOfCancels == 1 {
				ancli.PrintWarn("graceful shutdown ongoing, cancel again to force shutdown\n")
			} else {
				ancli.PrintErr("forcing shutdown\n")
				os.Exit(1)
			}
			amountOfCancels++
		}
	}
}

// MonitorV2 is the same as Monitor except for two points:
// 1. It breaks on ctx cancel
// 2. It doesn't append newline on the warns or error prints
func MonitorV2(ctx context.Context, cancel context.CancelFunc) {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	amountOfCancels := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-signalCh:
			if amountOfCancels == 0 {
				ancli.PrintWarn("initiating shutdown")
				cancel()
			} else if amountOfCancels == 1 {
				ancli.PrintWarn("graceful shutdown ongoing, cancel again to force shutdown")
			} else {
				ancli.PrintErr("forcing shutdown")
				os.Exit(1)
			}
			amountOfCancels++
		}
	}
}
