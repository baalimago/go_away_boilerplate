package cmd

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"testing"

	"github.com/baalimago/go_away_boilerplate/pkg/table"
	"github.com/baalimago/go_away_boilerplate/pkg/testboil"
)

type silentCause struct{ code int }

func (e silentCause) Error() string { return fmt.Sprintf("cause %d", e.code) }

func runSilentQuietly(t *testing.T, args []string, commands map[string]Command) (int, error) {
	t.Helper()
	var status int
	var err error
	var stderr string
	stdout := testboil.CaptureStdout(t, func(t *testing.T) {
		stderr = testboil.CaptureStderr(t, func(t *testing.T) {
			status, err = RunSilent(context.Background(), args, commands)
		})
	})
	if stdout != "" || stderr != "" {
		t.Fatalf("RunSilent printed stdout %q, stderr %q; want nothing", stdout, stderr)
	}
	return status, err
}

func Test_RunSilent_success(t *testing.T) {
	cmd := newTestCommand(
		func(context.Context) error { return nil },
		func(context.Context) error { return nil })

	status, err := runSilentQuietly(t, []string{"bin", "test"}, map[string]Command{"test": cmd})

	if status != 0 || err != nil {
		t.Fatalf("RunSilent() = %v, %v; want 0, nil", status, err)
	}
}

func Test_RunSilent_failuresKeepTheirCause(t *testing.T) {
	cause := silentCause{code: 7}
	nilStage := func(context.Context) error { return nil }
	failing := func(context.Context) error { return fmt.Errorf("wrapped: %w", cause) }
	tests := []struct {
		name  string
		setup func(context.Context) error
		run   func(context.Context) error
	}{
		{name: "setup", setup: failing, run: nilStage},
		{name: "run", setup: nilStage, run: failing},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newTestCommand(tc.setup, tc.run)

			status, err := runSilentQuietly(t, []string{"bin", "test"}, map[string]Command{"test": cmd})

			if status != 1 {
				t.Fatalf("status = %v, want 1", status)
			}
			var got silentCause
			if !errors.As(err, &got) || got != cause {
				t.Fatalf("errors.As(%v) did not reach the cause %v", err, cause)
			}
			if err.Error() != "wrapped: cause 7" {
				t.Fatalf("err.Error() = %q, want the cause's own text", err.Error())
			}
		})
	}
}

func Test_RunSilent_userInitiatedExit(t *testing.T) {
	for _, sentinel := range []error{ErrUserInitiatedExit, table.ErrUserInitiatedExit} {
		t.Run(sentinel.Error(), func(t *testing.T) {
			cmd := newTestCommand(
				func(context.Context) error { return nil },
				func(context.Context) error { return fmt.Errorf("quit: %w", sentinel) })

			status, err := runSilentQuietly(t, []string{"bin", "test"}, map[string]Command{"test": cmd})

			if status != 0 {
				t.Fatalf("status = %v, want 0", status)
			}
			if !errors.Is(err, sentinel) {
				t.Fatalf("err = %v, want one matching %v", err, sentinel)
			}
		})
	}
}

func Test_RunSilent_noArgs(t *testing.T) {
	status, err := runSilentQuietly(t, []string{"bin"}, map[string]Command{})

	if status != 1 || !errors.Is(err, ErrNoArgs) {
		t.Fatalf("RunSilent() = %v, %v; want 1, ErrNoArgs", status, err)
	}
}

func Test_RunSilent_helpRequested(t *testing.T) {
	cmd := newTestCommand(nil, nil)
	cmd.helpFunc = func() string { return "test help" }

	status, err := runSilentQuietly(t, []string{"bin", "test", "-h"}, map[string]Command{"test": cmd})

	if status != 0 {
		t.Fatalf("status = %v, want 0", status)
	}
	var help HelpRequestedError
	if !errors.As(err, &help) {
		t.Fatalf("err = %v, want a HelpRequestedError", err)
	}
	if help.Help != "test help" {
		t.Fatalf("Help = %q, want the command's help text", help.Help)
	}
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("err = %v, want it to match flag.ErrHelp", err)
	}
}

func Test_RunSilent_dispatchErrorsReturned(t *testing.T) {
	cmd := newTestCommand(nil, nil)

	status, err := runSilentQuietly(t, []string{"bin", "nope"}, map[string]Command{"test": cmd})

	if status != 1 {
		t.Fatalf("status = %v, want 1", status)
	}
	if _, ok := errors.AsType[ArgNotFoundError](err); !ok {
		t.Fatalf("err = %v, want an ArgNotFoundError", err)
	}
}

func Test_Run_unchangedOutputOnFailure(t *testing.T) {
	cmd := newTestCommand(
		func(context.Context) error { return nil },
		func(context.Context) error { return silentCause{code: 3} })

	var status int
	stderr := testboil.CaptureStderr(t, func(t *testing.T) {
		status = Run(context.Background(), []string{"bin", "test"}, map[string]Command{"test": cmd}, "usage: %v")
	})

	if status != 1 {
		t.Fatalf("Run() = %v, want 1", status)
	}
	testboil.AssertStringContains(t, stderr, "failed to run: cause 3")
}

func runSilentToQuietly(t *testing.T, args []string, commands map[string]Command) (int, string, error) {
	t.Helper()
	var status int
	var err error
	var stderr string
	var out bytes.Buffer
	stdout := testboil.CaptureStdout(t, func(t *testing.T) {
		stderr = testboil.CaptureStderr(t, func(t *testing.T) {
			status, err = RunSilentTo(context.Background(), args, commands, &out)
		})
	})
	if stdout != "" || stderr != "" {
		t.Fatalf("RunSilentTo printed stdout %q, stderr %q; want nothing", stdout, stderr)
	}
	return status, out.String(), err
}

func Test_RunSilentTo_completionCandidatesGoToOut(t *testing.T) {
	cmd := newTestCommand(
		func(context.Context) error { return nil },
		func(context.Context) error { return nil })

	status, out, err := runSilentToQuietly(t, []string{"bin", "__complete", "bin", "te"}, map[string]Command{"test": cmd})

	if status != 0 || err != nil {
		t.Fatalf("RunSilentTo() = %v, %v; want 0, nil", status, err)
	}
	if out != "test\tplain\n" {
		t.Fatalf("out = %q, want the candidate line", out)
	}
}

func Test_RunSilentTo_completionScriptGoesToOut(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			status, out, err := runSilentToQuietly(t, []string{"bin", "completion", shell}, map[string]Command{})

			if status != 0 || err != nil {
				t.Fatalf("RunSilentTo() = %v, %v; want 0, nil", status, err)
			}
			want := bashCompletionScript("bin")
			if shell == "zsh" {
				want = zshCompletionScript("bin")
			}
			if out != want {
				t.Fatalf("out = %q, want the %s script", out, shell)
			}
		})
	}
}

func Test_RunSilentTo_otherOutcomesMatchRunSilent(t *testing.T) {
	cause := silentCause{code: 5}
	cmd := newTestCommand(
		func(context.Context) error { return nil },
		func(context.Context) error { return cause })

	status, out, err := runSilentToQuietly(t, []string{"bin", "test"}, map[string]Command{"test": cmd})

	if status != 1 || !errors.Is(err, cause) || out != "" {
		t.Fatalf("RunSilentTo() = %v, %v, out %q; want 1, the cause, no output", status, err, out)
	}
}

func Test_RunSilent_completionCandidatesGoToStdout(t *testing.T) {
	cmd := newTestCommand(
		func(context.Context) error { return nil },
		func(context.Context) error { return nil })

	var status int
	var err error
	stdout := testboil.CaptureStdout(t, func(t *testing.T) {
		status, err = RunSilent(context.Background(), []string{"bin", "__complete", "bin", "te"}, map[string]Command{"test": cmd})
	})

	if status != 0 || err != nil || stdout != "test\tplain\n" {
		t.Fatalf("RunSilent() = %v, %v, stdout %q; want 0, nil, the candidate line", status, err, stdout)
	}
}

func Test_IsCommandFailure(t *testing.T) {
	nilStage := func(context.Context) error { return nil }
	failing := func(context.Context) error { return silentCause{code: 3} }
	tests := []struct {
		name     string
		args     []string
		commands map[string]Command
		want     bool
	}{
		{name: "setup failure", args: []string{"bin", "test"}, commands: map[string]Command{"test": newTestCommand(failing, nilStage)}, want: true},
		{name: "run failure", args: []string{"bin", "test"}, commands: map[string]Command{"test": newTestCommand(nilStage, failing)}, want: true},
		{name: "built-in completion with an unsupported shell", args: []string{"bin", "completion", "fish"}, commands: map[string]Command{}, want: true},
		{name: "built-in completion without a shell", args: []string{"bin", "completion"}, commands: map[string]Command{}, want: true},
		{name: "unknown command", args: []string{"bin", "nope"}, commands: map[string]Command{"test": newTestCommand(nil, nil)}, want: false},
		{name: "no command", args: []string{"bin"}, commands: map[string]Command{}, want: false},
		{name: "undefined flag", args: []string{"bin", "test", "-zzz"}, commands: map[string]Command{"test": newTestCommand(nil, nil)}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runSilentToQuietly(t, tc.args, tc.commands)
			if err == nil {
				t.Fatal("RunSilentTo() returned no error")
			}
			if got := IsCommandFailure(err); got != tc.want {
				t.Fatalf("IsCommandFailure(%v) = %v, want %v", err, got, tc.want)
			}
		})
	}
	if IsCommandFailure(nil) {
		t.Fatal("IsCommandFailure(nil) = true, want false")
	}
	if IsCommandFailure(silentCause{code: 1}) {
		t.Fatal("IsCommandFailure of an error RunSilent did not return = true, want false")
	}
}
