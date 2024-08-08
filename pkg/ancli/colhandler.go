package ancli

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
)

type ansiprint struct{}

func (a *ansiprint) Enabled(context.Context, slog.Level) bool {
	return true
}

func (a *ansiprint) Handle(ctx context.Context, r slog.Record) error {
	var bf bytes.Buffer

	if !r.Time.IsZero() {
		fmt.Fprintf(&bf, "%v %v", r.Time, r.Message)
	}

	switch r.Level {
	case slog.LevelDebug:
		printStatus(os.Stdout, "debug", bf.String(), BLUE)
	case slog.LevelInfo:
		printStatus(os.Stdout, "ok", bf.String(), CYAN)
	case slog.LevelWarn:
		printStatus(os.Stdout, "warning", bf.String(), YELLOW)
	case slog.LevelError:
		printStatus(os.Stdout, "error", bf.String(), RED)
	}
	return nil
}

func (a *ansiprint) WithAttrs(attrs []slog.Attr) slog.Handler {
	return a
}

func (a *ansiprint) WithGroup(name string) slog.Handler {
	return a
}
