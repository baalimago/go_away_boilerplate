package cmd

import (
	"context"
	"errors"
	"flag"
	"strings"
	"testing"
)

// scanCommands returns fresh q|query (String cm, Bool re) + serve (no flags)
// commands per call, since Parse state persists on memoized flagsets.
func scanCommands() (map[string]Command, *flag.FlagSet) {
	qFs := flag.NewFlagSet("query", flag.ContinueOnError)
	qFs.String("cm", "", "chat model")
	qFs.Bool("re", false, "use replies")
	q := &mockCommand{
		describeFunc: func() string { return "query" },
		flagSet:      qFs,
	}
	serve := &mockCommand{
		describeFunc: func() string { return "serve" },
		flagSet:      flag.NewFlagSet("serve", flag.ContinueOnError),
	}
	return map[string]Command{"q|query": q, "serve": serve}, qFs
}

func Test_parse_arityAwareScan(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantCommand    string
		wantCm         string
		wantRe         bool
		wantPositional []string
	}{
		{
			name:           "value flag, space form, before command",
			args:           []string{"-cm", "gpt-4", "q", "hi"},
			wantCommand:    "query",
			wantCm:         "gpt-4",
			wantPositional: []string{"hi"},
		},
		{
			name:           "bool flag before command",
			args:           []string{"-re", "q", "hi"},
			wantCommand:    "query",
			wantRe:         true,
			wantPositional: []string{"hi"},
		},
		{
			name:           "= form before command",
			args:           []string{"-cm=gpt-4", "q"},
			wantCommand:    "query",
			wantCm:         "gpt-4",
			wantPositional: []string{},
		},
		{
			name:           "flags after command",
			args:           []string{"q", "-cm", "gpt-4", "hi"},
			wantCommand:    "query",
			wantCm:         "gpt-4",
			wantPositional: []string{"hi"},
		},
		{
			name:           "double-dash terminator",
			args:           []string{"-re", "--", "q", "-cm"},
			wantCommand:    "query",
			wantRe:         true,
			wantPositional: []string{"-cm"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands, qFs := scanCommands()
			got, err := parse(append([]string{"bin"}, tt.args...), commands)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if got.Describe() != tt.wantCommand {
				t.Fatalf("command = %v, want %v", got.Describe(), tt.wantCommand)
			}
			if gotCm := qFs.Lookup("cm").Value.String(); gotCm != tt.wantCm {
				t.Errorf("cm = %q, want %q", gotCm, tt.wantCm)
			}
			if gotRe := qFs.Lookup("re").Value.String() == "true"; gotRe != tt.wantRe {
				t.Errorf("re = %v, want %v", gotRe, tt.wantRe)
			}
			gotPos := qFs.Args()
			if len(gotPos) != len(tt.wantPositional) {
				t.Fatalf("positional = %v, want %v", gotPos, tt.wantPositional)
			}
			for i := range gotPos {
				if gotPos[i] != tt.wantPositional[i] {
					t.Fatalf("positional = %v, want %v", gotPos, tt.wantPositional)
				}
			}
		})
	}
}

func Test_parse_scanErrors(t *testing.T) {
	t.Run("bare dash is command candidate", func(t *testing.T) {
		commands, _ := scanCommands()
		_, err := parse([]string{"bin", "-"}, commands)
		want := ArgNotFoundError("-")
		if !errors.As(err, &want) || want != ArgNotFoundError("-") {
			t.Fatalf("err = %v, want ArgNotFoundError(\"-\")", err)
		}
	})

	t.Run("unknown bool-arity flag before command still resolves, flagset errors", func(t *testing.T) {
		commands, _ := scanCommands()
		got, err := parse([]string{"bin", "-nope", "q"}, commands)
		if got == nil || got.Describe() != "query" {
			t.Fatalf("expected query command resolved, got: %v", got)
		}
		if err == nil || !strings.Contains(err.Error(), "failed to parse flagset") {
			t.Fatalf("expected flagset parse error, got: %v", err)
		}
	})

	t.Run("value flag at end, no command", func(t *testing.T) {
		commands, _ := scanCommands()
		_, err := parse([]string{"bin", "-cm", "gpt-4"}, commands)
		if !errors.Is(err, ErrNoArgs) {
			t.Fatalf("err = %v, want ErrNoArgs", err)
		}
	})

	t.Run("only unknown bool flags, no command", func(t *testing.T) {
		commands, _ := scanCommands()
		_, err := parse([]string{"bin", "-foo", "-r"}, commands)
		if !errors.Is(err, ErrNoArgs) {
			t.Fatalf("err = %v, want ErrNoArgs", err)
		}
	})

	t.Run("unknown candidate names the candidate token", func(t *testing.T) {
		commands, _ := scanCommands()
		_, err := parse([]string{"bin", "-cm", "gpt-4", "qq"}, commands)
		want := ArgNotFoundError("qq")
		if !errors.As(err, &want) || want != ArgNotFoundError("qq") {
			t.Fatalf("err = %v, want ArgNotFoundError(\"qq\")", err)
		}
	})

	t.Run("arity conflict resolves as value-taking", func(t *testing.T) {
		aFs := flag.NewFlagSet("A", flag.ContinueOnError)
		aFs.Bool("x", false, "")
		bFs := flag.NewFlagSet("B", flag.ContinueOnError)
		bFs.String("x", "", "")
		commands := map[string]Command{
			"A": &mockCommand{describeFunc: func() string { return "A" }, flagSet: aFs},
			"B": &mockCommand{describeFunc: func() string { return "B" }, flagSet: bFs},
		}
		_, err := parse([]string{"bin", "-x", "A"}, commands)
		if !errors.Is(err, ErrNoArgs) {
			t.Fatalf("err = %v, want ErrNoArgs (A consumed as value of -x)", err)
		}
	})

	t.Run("nil flagset on any registered command errors without panic", func(t *testing.T) {
		commands := map[string]Command{
			"ok": &mockCommand{
				describeFunc: func() string { return "ok" },
				flagSet:      flag.NewFlagSet("ok", flag.ContinueOnError),
			},
			"broken": &mockCommand{
				describeFunc: func() string { return "broken" },
				flagSet:      nil,
			},
		}
		_, err := parse([]string{"bin", "ok"}, commands)
		if err == nil || !strings.Contains(err.Error(), "flagset is nil") {
			t.Fatalf("expected nil-flagset error, got: %v", err)
		}
	})
}

func Test_Run_arityAwareScan(t *testing.T) {
	for _, args := range [][]string{
		{"bin", "-cm", "gpt-4", "q", "hi"},
		{"bin", "q", "-cm", "gpt-4", "hi"},
	} {
		t.Run(strings.Join(args[1:], " "), func(t *testing.T) {
			commands, qFs := scanCommands()
			ran := false
			commands["q|query"].(*mockCommand).setupFunc = func() error { return nil }
			commands["q|query"].(*mockCommand).runFunc = func(context.Context) error {
				ran = true
				if got := qFs.Lookup("cm").Value.String(); got != "gpt-4" {
					t.Errorf("cm = %q, want %q", got, "gpt-4")
				}
				if got := qFs.Args(); len(got) != 1 || got[0] != "hi" {
					t.Errorf("positional = %v, want [hi]", got)
				}
				return nil
			}
			if code := Run(context.Background(), args, commands, "usage: %v"); code != 0 {
				t.Fatalf("Run() = %v, want 0", code)
			}
			if !ran {
				t.Fatal("expected command Run to be called")
			}
		})
	}
}
