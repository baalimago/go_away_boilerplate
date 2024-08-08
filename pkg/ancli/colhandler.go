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
		fmt.Fprintf(&bf, "%v %v", r.Time.Format("2006-01-02T15:04Z"), r.Message)
	}

	switch r.Level {
	case slog.LevelDebug, slog.LevelWarn, slog.LevelInfo:
		fmt.Fprint(os.Stdout, bf.String())
	case slog.LevelError:
		fmt.Fprint(os.Stderr, bf.String())
	}
	return nil
}

func (a *ansiprint) WithAttrs(attrs []slog.Attr) slog.Handler {
	return a
}

func (a *ansiprint) WithGroup(name string) slog.Handler {
	return a
}
