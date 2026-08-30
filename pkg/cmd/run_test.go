package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"testing"

	"github.com/baalimago/go_away_boilerplate/pkg/table"
	"github.com/baalimago/go_away_boilerplate/pkg/testboil"
)

type ctxKey string

func newTestCommand(setup func(context.Context) error, run func(context.Context) error) *mockCommand {
	return &mockCommand{
		describeFunc: func() string { return "test" },
		setupCtxFunc: setup,
		runFunc:      run,
		flagSet:      flag.NewFlagSet("test", flag.ContinueOnError),
	}
}

func Test_Run_ctxPassthrough(t *testing.T) {
	key := ctxKey("testKey")
	want := "testValue"
	ctx := context.WithValue(context.Background(), key, want)
	var setupCtx, runCtx context.Context
	cmd := newTestCommand(
		func(ctx context.Context) error {
			setupCtx = ctx
			return nil
		},
		func(ctx context.Context) error {
			runCtx = ctx
			return nil
		})

	got := Run(ctx, []string{"bin", "test"}, map[string]Command{"test": cmd}, "usage: %v")

	if got != 0 {
		t.Fatalf("Run() = %v, want 0", got)
	}
	if setupCtx.Value(key) != want {
		t.Fatalf("Setup ctx value = %v, want %v", setupCtx.Value(key), want)
	}
	if runCtx != setupCtx {
		t.Fatalf("Run ctx = %v, want same as Setup ctx: %v", runCtx, setupCtx)
	}
}

func Test_Run_canceledCtxUnaltered(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var setupErr, runErr error
	cmd := newTestCommand(
		func(ctx context.Context) error {
			setupErr = ctx.Err()
			return nil
		},
		func(ctx context.Context) error {
			runErr = ctx.Err()
			return nil
		})

	Run(ctx, []string{"bin", "test"}, map[string]Command{"test": cmd}, "usage: %v")

	if !errors.Is(setupErr, context.Canceled) {
		t.Fatalf("Setup ctx.Err() = %v, want context.Canceled", setupErr)
	}
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("Run ctx.Err() = %v, want context.Canceled", runErr)
	}
}

func Test_Run_userInitiatedExit(t *testing.T) {
	nilSetup := func(context.Context) error { return nil }
	tests := []struct {
		name string
		cmd  *mockCommand
	}{
		{
			name: "cmd sentinel from Setup",
			cmd: newTestCommand(
				func(context.Context) error { return ErrUserInitiatedExit },
				func(context.Context) error {
					t.Error("Run should not be called after Setup exit")
					return nil
				}),
		},
		{
			name: "wrapped cmd sentinel from Run",
			cmd: newTestCommand(nilSetup, func(context.Context) error {
				return fmt.Errorf("x: %w", ErrUserInitiatedExit)
			}),
		},
		{
			name: "table sentinel from Setup",
			cmd: newTestCommand(func(context.Context) error {
				return table.ErrUserInitiatedExit
			}, nil),
		},
		{
			name: "wrapped table sentinel from Run",
			cmd: newTestCommand(nilSetup, func(context.Context) error {
				return fmt.Errorf("x: %w", table.ErrUserInitiatedExit)
			}),
		},
		{
			name: "double-wrapped cmd sentinel from Run",
			cmd: newTestCommand(nilSetup, func(context.Context) error {
				return fmt.Errorf("y: %w", fmt.Errorf("x: %w", ErrUserInitiatedExit))
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotOut string
			gotErr := testboil.CaptureStderr(t, func(t *testing.T) {
				gotOut = testboil.CaptureStdout(t, func(t *testing.T) {
					code := Run(context.Background(), []string{"bin", "test"},
						map[string]Command{"test": tt.cmd}, "usage: %v")
					if code != 0 {
						t.Errorf("Run() = %v, want 0", code)
					}
				})
			})
			if gotOut != "" {
				t.Errorf("expected no stdout, got: %q", gotOut)
			}
			if gotErr != "" {
				t.Errorf("expected no stderr, got: %q", gotErr)
			}
		})
	}
}

func Test_Run_ordinaryErrorStillFails(t *testing.T) {
	cmd := newTestCommand(
		func(context.Context) error { return nil },
		func(context.Context) error { return errors.New("boom") })

	var code int
	gotErr := testboil.CaptureStderr(t, func(t *testing.T) {
		code = Run(context.Background(), []string{"bin", "test"},
			map[string]Command{"test": cmd}, "usage: %v")
	})

	if code != 1 {
		t.Fatalf("Run() = %v, want 1", code)
	}
	if !strings.Contains(gotErr, "boom") {
		t.Fatalf("expected stderr to contain 'boom', got: %q", gotErr)
	}
}

// Test_Run_flagsetOutputSilenced pins that the dispatcher is the only
// voice: stdlib's "Usage of x:" dump and error echo never print, neither
// on -h (the command's Help() prints instead) nor on a bad flag (the
// dispatcher's error prints instead).
func Test_Run_flagsetOutputSilenced(t *testing.T) {
	newCmds := func() map[string]Command {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.String("cm", "", "some flag")
		return map[string]Command{"test": &mockCommand{
			describeFunc: func() string { return "test" },
			helpFunc:     func() string { return "test help text" },
			setupFunc:    func() error { return nil },
			runFunc:      func(context.Context) error { return nil },
			flagSet:      fs,
		}}
	}

	t.Run("-h prints Help() without stdlib usage dump", func(t *testing.T) {
		var code int
		var gotOut string
		gotErr := testboil.CaptureStderr(t, func(t *testing.T) {
			gotOut = testboil.CaptureStdout(t, func(t *testing.T) {
				code = Run(context.Background(), []string{"bin", "test", "-h"}, newCmds(), "usage: %v")
			})
		})
		if code != 0 {
			t.Fatalf("Run() = %v, want 0", code)
		}
		if !strings.Contains(gotOut, "test help text") {
			t.Fatalf("expected Help() on stdout, got: %q", gotOut)
		}
		for name, out := range map[string]string{"stdout": gotOut, "stderr": gotErr} {
			if strings.Contains(out, "Usage of") {
				t.Fatalf("stdlib usage dump leaked to %v: %q", name, out)
			}
		}
	})

	t.Run("bad flag prints only the dispatcher error", func(t *testing.T) {
		var code int
		gotErr := testboil.CaptureStderr(t, func(t *testing.T) {
			_ = testboil.CaptureStdout(t, func(t *testing.T) {
				code = Run(context.Background(), []string{"bin", "test", "-bogus"}, newCmds(), "usage: %v")
			})
		})
		if code != 1 {
			t.Fatalf("Run() = %v, want 1", code)
		}
		if !strings.Contains(gotErr, "failed to parse flagset") {
			t.Fatalf("expected dispatcher error, got: %q", gotErr)
		}
		if strings.Contains(gotErr, "Usage of") {
			t.Fatalf("stdlib usage dump leaked: %q", gotErr)
		}
	})
}

func Test_formatCommandDescriptions_sorted(t *testing.T) {
	commands := map[string]Command{}
	for _, name := range []string{"zeta", "alpha", "mid|m", "beta"} {
		commands[name] = &mockCommand{describeFunc: func() string { return "desc " + name }}
	}

	first := formatCommandDescriptions(commands)
	for range 10 {
		if got := formatCommandDescriptions(commands); got != first {
			t.Fatalf("output not stable, first: %q, got: %q", first, got)
		}
	}

	var lastIdx int
	for _, name := range []string{"alpha", "beta", "mid|m", "zeta"} {
		idx := strings.Index(first, name)
		if idx == -1 {
			t.Fatalf("expected output to contain %q, got: %q", name, first)
		}
		if idx < lastIdx {
			t.Fatalf("expected lexicographic order, %q out of place in: %q", name, first)
		}
		lastIdx = idx
	}
}
