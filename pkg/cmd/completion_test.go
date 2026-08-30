package cmd

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baalimago/go_away_boilerplate/pkg/testboil"
)

type mockCompleterCommand struct {
	mockCommand
	flagValues func(flagName, partial string) []CompletionItem
	argValues  func(args []string, partial string) []CompletionItem
}

func (m *mockCompleterCommand) CompleteFlagValue(flagName, partial string) []CompletionItem {
	return m.flagValues(flagName, partial)
}

func (m *mockCompleterCommand) CompleteArgs(args []string, partial string) []CompletionItem {
	return m.argValues(args, partial)
}

// completionTree builds the phase-3 mock tree where every Setup/Run panics,
// proving engine purity across the whole suite.
func completionTree() map[string]Command {
	panicSetup := func() error { panic("Setup called during completion") }
	panicRun := func(context.Context) error { panic("Run called during completion") }

	chatFs := flag.NewFlagSet("chat", flag.ContinueOnError)
	chatFs.Bool("r", false, "")
	listFs := flag.NewFlagSet("list", flag.ContinueOnError)
	listFs.Bool("x", false, "")
	chat := &mockSubcommander{
		mockCommand: mockCommand{
			describeFunc: func() string { return "manage chats" },
			setupFunc:    panicSetup,
			runFunc:      panicRun,
			flagSet:      chatFs,
		},
		subs: map[string]Command{
			"list|l": &mockCommand{
				describeFunc: func() string { return "list chats" },
				setupFunc:    panicSetup,
				runFunc:      panicRun,
				flagSet:      listFs,
			},
			"del": &mockCommand{
				describeFunc: func() string { return "delete chats" },
				setupFunc:    panicSetup,
				runFunc:      panicRun,
				flagSet:      flag.NewFlagSet("del", flag.ContinueOnError),
			},
		},
	}

	qFs := flag.NewFlagSet("query", flag.ContinueOnError)
	qFs.String("cm", "", "")
	qFs.String("f", "", "")
	q := &mockCompleterCommand{
		mockCommand: mockCommand{
			describeFunc: func() string { return "query" },
			setupFunc:    panicSetup,
			runFunc:      panicRun,
			flagSet:      qFs,
		},
		flagValues: func(flagName, partial string) []CompletionItem {
			switch flagName {
			case "cm":
				var out []CompletionItem
				for _, m := range []string{"m1", "m2"} {
					if strings.HasPrefix(m, partial) {
						out = append(out, CompletionItem{Value: m, Kind: CompletionKindPlain})
					}
				}
				return out
			case "f":
				return []CompletionItem{{Value: "x", Kind: CompletionKindFile}}
			}
			return nil
		},
		argValues: func(args []string, partial string) []CompletionItem {
			return []CompletionItem{}
		},
	}

	return map[string]Command{"chat|c": chat, "q|query": q}
}

func runCompletion(t *testing.T, commands map[string]Command, argv ...string) (code int, stdout, stderr string) {
	t.Helper()
	stderr = testboil.CaptureStderr(t, func(t *testing.T) {
		stdout = testboil.CaptureStdout(t, func(t *testing.T) {
			code = Run(context.Background(), argv, commands, "usage: %v")
		})
	})
	return code, stdout, stderr
}

func completeLines(t *testing.T, words ...string) []string {
	t.Helper()
	argv := append([]string{"myapp", "__complete", "myapp"}, words...)
	code, stdout, stderr := runCompletion(t, completionTree(), argv...)
	if code != 0 {
		t.Fatalf("__complete exit = %v, want 0 (stderr: %q)", code, stderr)
	}
	if stdout == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
}

func Test_complete_suggestions(t *testing.T) {
	tests := []struct {
		name      string
		words     []string
		want      []string
		forbidden []string
	}{
		{
			name:  "empty position lists commands and flags",
			words: []string{""},
			want: []string{
				"c\tplain", "chat\tplain", "completion\tplain", "q\tplain", "query\tplain",
				"-cm\tplain", "-f\tplain", "-r\tplain",
			},
			forbidden: []string{"__complete"},
		},
		{
			name:  "command prefix",
			words: []string{"ch"},
			want:  []string{"chat\tplain"},
		},
		{
			name:      "flag names for resolved command",
			words:     []string{"q", "-"},
			want:      []string{"-cm\tplain", "-f\tplain"},
			forbidden: []string{"-r\tplain"},
		},
		{
			name:  "flag value via hook",
			words: []string{"q", "-cm", ""},
			want:  []string{"m1\tplain", "m2\tplain"},
		},
		{
			name:      "flag value prefix filtered by hook",
			words:     []string{"q", "-cm", "m1"},
			want:      []string{"m1\tplain"},
			forbidden: []string{"m2\tplain"},
		},
		{
			name:      "subcommand names",
			words:     []string{"chat", ""},
			want:      []string{"del\tplain", "l\tplain", "list\tplain"},
			forbidden: []string{"q\tplain", "query\tplain", "completion\tplain"},
		},
		{
			name:  "arg completer suppression",
			words: []string{"q", "hello", ""},
			want:  nil,
		},
		{
			name:  "file kind passthrough",
			words: []string{"q", "-f", ""},
			want:  []string{"x\tfile"},
		},
		{
			name:  "unknown command prefix falls back to top level",
			words: []string{"bogus", "ch"},
			want:  []string{"chat\tplain"},
		},
		{
			name:  "value flag before any command gives nothing",
			words: []string{"-cm", "gp"},
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := completeLines(t, tt.words...)
			if tt.want == nil {
				if len(lines) != 0 {
					t.Fatalf("expected no lines, got: %v", lines)
				}
				return
			}
			if len(lines) != len(tt.want) {
				t.Fatalf("lines = %v, want %v", lines, tt.want)
			}
			for i := range tt.want {
				if lines[i] != tt.want[i] {
					t.Fatalf("line %d = %q, want %q (all: %v)", i, lines[i], tt.want[i], lines)
				}
			}
			for _, f := range tt.forbidden {
				for _, l := range lines {
					if strings.Contains(l, f) {
						t.Fatalf("forbidden %q found in %v", f, lines)
					}
				}
			}
		})
	}
}

type mockCompleterSubcommander struct {
	mockSubcommander
	argValues func(args []string, partial string) []CompletionItem
}

func (m *mockCompleterSubcommander) CompleteArgs(args []string, partial string) []CompletionItem {
	return m.argValues(args, partial)
}

func Test_complete_subcommanderArgMerge(t *testing.T) {
	tree := completionTree()
	chat := tree["chat|c"].(*mockSubcommander)
	tree["chat|c"] = &mockCompleterSubcommander{
		mockSubcommander: *chat,
		argValues: func(args []string, partial string) []CompletionItem {
			if strings.HasPrefix("zextra", partial) {
				return []CompletionItem{{Value: "zextra", Kind: CompletionKindPlain}}
			}
			return []CompletionItem{}
		},
	}
	code, stdout, _ := runCompletion(t, tree, "myapp", "__complete", "myapp", "chat", "")
	if code != 0 {
		t.Fatalf("exit = %v, want 0", code)
	}
	want := "del\tplain\nl\tplain\nlist\tplain\nzextra\tplain\n"
	if stdout != want {
		t.Fatalf("expected subcommand names + arg completions, got: %q, want %q", stdout, want)
	}
}

func Test_complete_builtinCompletionShells(t *testing.T) {
	for words, want := range map[string]string{
		"":     "bash\tplain\nzsh\tplain\n",
		"b":    "bash\tplain\n",
		"bash": "bash\tplain\n",
	} {
		code, stdout, _ := runCompletion(t, completionTree(),
			"myapp", "__complete", "myapp", "completion", words)
		if code != 0 {
			t.Fatalf("exit = %v, want 0", code)
		}
		if stdout != want {
			t.Fatalf("words %q: got %q, want %q", words, stdout, want)
		}
	}

	t.Run("nothing after a chosen shell", func(t *testing.T) {
		code, stdout, _ := runCompletion(t, completionTree(),
			"myapp", "__complete", "myapp", "completion", "bash", "")
		if code != 0 || stdout != "" {
			t.Fatalf("code=%v stdout=%q, want 0 and empty", code, stdout)
		}
	})
}

func Test_complete_edgeCases(t *testing.T) {
	t.Run("no words at all", func(t *testing.T) {
		code, stdout, _ := runCompletion(t, completionTree(), "myapp", "__complete")
		if code != 0 || stdout != "" {
			t.Fatalf("code=%v stdout=%q, want 0 and empty", code, stdout)
		}
	})

	t.Run("panicking hook recovers to empty exit 0", func(t *testing.T) {
		commands := completionTree()
		q := commands["q|query"].(*mockCompleterCommand)
		q.flagValues = func(string, string) []CompletionItem { panic("hook boom") }
		q.argValues = func([]string, string) []CompletionItem { panic("hook boom") }
		for _, words := range [][]string{
			{"q", "-cm", ""},
			{"q", "hello", ""},
		} {
			code, stdout, _ := runCompletion(t, commands,
				append([]string{"myapp", "__complete", "myapp"}, words...)...)
			if code != 0 || stdout != "" {
				t.Fatalf("words %v: code=%v stdout=%q, want 0 and empty", words, code, stdout)
			}
		}
	})
}

func Test_completion_scripts(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			code, stdout, _ := runCompletion(t, completionTree(), "myapp", "completion", shell)
			if code != 0 {
				t.Fatalf("exit = %v, want 0", code)
			}
			if !strings.Contains(stdout, "myapp __complete") {
				t.Fatalf("script not wired to binary basename: %q", stdout)
			}
			if strings.Contains(stdout, "clai") {
				t.Fatalf("script contains hardcoded app name: %q", stdout)
			}
			registration := map[string]string{"bash": "complete -F", "zsh": "compdef"}[shell]
			if !strings.Contains(stdout, registration) {
				t.Fatalf("script missing %q registration: %q", registration, stdout)
			}

			if _, err := exec.LookPath(shell); err != nil {
				t.Skipf("%v not installed; script syntax not checked", shell)
			}
			f := filepath.Join(t.TempDir(), "script."+shell)
			if err := os.WriteFile(f, []byte(stdout), 0o644); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(shell, "-n", f).CombinedOutput(); err != nil {
				t.Fatalf("%v -n failed: %v\n%s", shell, err, out)
			}
		})
	}

	t.Run("unsupported shell", func(t *testing.T) {
		code, stdout, stderr := runCompletion(t, completionTree(), "myapp", "completion", "fish")
		if code != 1 {
			t.Fatalf("exit = %v, want 1", code)
		}
		if stdout != "" {
			t.Fatalf("stdout must stay clean, got: %q", stdout)
		}
		if strings.Count(strings.TrimSuffix(stderr, "\n"), "\n") != 0 {
			t.Fatalf("want a single stderr line, got: %q", stderr)
		}
	})

	t.Run("missing shell argument", func(t *testing.T) {
		code, _, stderr := runCompletion(t, completionTree(), "myapp", "completion")
		if code != 1 {
			t.Fatalf("exit = %v, want 1", code)
		}
		if !strings.Contains(stderr, "completion <bash|zsh>") {
			t.Fatalf("expected usage hint on stderr, got: %q", stderr)
		}
	})
}

func Test_completion_autoRegistration(t *testing.T) {
	t.Run("caller map not mutated and completion listed in usage", func(t *testing.T) {
		commands := completionTree()
		before := len(commands)
		_, stdout, _ := runCompletion(t, commands, "myapp")
		if len(commands) != before {
			t.Fatalf("caller's map mutated: %d entries, was %d", len(commands), before)
		}
		if !strings.Contains(stdout, "completion") {
			t.Fatalf("usage should list completion, got: %q", stdout)
		}
		if strings.Contains(stdout, "__complete") {
			t.Fatalf("usage must not list __complete, got: %q", stdout)
		}
	})

	t.Run("app-defined completion wins", func(t *testing.T) {
		ran := false
		commands := completionTree()
		commands["completion"] = &mockCommand{
			describeFunc: func() string { return "app completion" },
			setupFunc:    func() error { return nil },
			runFunc: func(context.Context) error {
				ran = true
				return nil
			},
			flagSet: flag.NewFlagSet("completion", flag.ContinueOnError),
		}
		code, _, _ := runCompletion(t, commands, "myapp", "completion", "bash")
		if code != 0 || !ran {
			t.Fatalf("code=%v appRan=%v, want app-defined completion to run", code, ran)
		}
	})

	t.Run("app-defined __complete wins", func(t *testing.T) {
		ran := false
		commands := completionTree()
		commands["__complete"] = &mockCommand{
			describeFunc: func() string { return "app complete" },
			setupFunc:    func() error { return nil },
			runFunc: func(context.Context) error {
				ran = true
				return nil
			},
			flagSet: flag.NewFlagSet("__complete", flag.ContinueOnError),
		}
		code, _, _ := runCompletion(t, commands, "myapp", "__complete", "myapp", "")
		if code != 0 || !ran {
			t.Fatalf("code=%v appRan=%v, want app-defined __complete to run", code, ran)
		}
	})
}
