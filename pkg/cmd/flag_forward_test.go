package cmd

import (
	"errors"
	"flag"
	"strings"
	"testing"
)

// forwardTree is hintTree plus an "audio"-level bool flag (-v) and a
// second, flagless "help|h" sub: enough shape to place a flag one level
// too shallow, one level too deep, and on a branch that is never resolved.
func forwardTree() (map[string]Command, *flag.FlagSet, *flag.FlagSet) {
	qFs := flag.NewFlagSet("query", flag.ContinueOnError)
	qFs.String("cm", "", "chat model")
	transcribeFs := flag.NewFlagSet("transcribe", flag.ContinueOnError)
	transcribeFs.String("am", "", "audio model")
	transcribeFs.Bool("x", false, "bool sub flag")
	audioFs := flag.NewFlagSet("audio", flag.ContinueOnError)
	audioFs.Bool("v", false, "verbose")
	audioFs.Int("par", 0, "parallelism")
	audio := &mockSubcommander{
		mockCommand: mockCommand{
			describeFunc: func() string { return "audio" },
			flagSet:      audioFs,
		},
		subs: map[string]Command{
			"transcribe|t": &mockCommand{
				describeFunc: func() string { return "transcribe" },
				flagSet:      transcribeFs,
			},
			"help|h": &mockCommand{
				describeFunc: func() string { return "help" },
				flagSet:      flag.NewFlagSet("help", flag.ContinueOnError),
			},
		},
	}
	query := &mockCommand{describeFunc: func() string { return "query" }, flagSet: qFs}
	return map[string]Command{"query|q": query, "audio|a": audio}, audioFs, transcribeFs
}

// Test_dispatch_forwardsToOwningLevel pins the placement convenience: a
// flag written at the wrong level reaches the level that defines it, as
// long as that level is on the resolved path. Placement is a convenience,
// not a rule.
func Test_dispatch_forwardsToOwningLevel(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantAm   string
		wantX    bool
		wantV    bool
		wantArgs []string
	}{
		{
			name:     "value flag before its verb",
			args:     []string{"app", "-am", "some-model", "a", "t", "hi"},
			wantAm:   "some-model",
			wantArgs: []string{"hi"},
		},
		{
			name:     "= form before its verb",
			args:     []string{"app", "-am=some-model", "a", "t", "hi"},
			wantAm:   "some-model",
			wantArgs: []string{"hi"},
		},
		{
			name:     "bool flag before its verb",
			args:     []string{"app", "-x", "a", "t", "hi"},
			wantX:    true,
			wantArgs: []string{"hi"},
		},
		{
			name:     "between the command and its verb",
			args:     []string{"app", "a", "-am", "some-model", "t", "hi"},
			wantAm:   "some-model",
			wantArgs: []string{"hi"},
		},
		{
			name:     "parent flag written after the verb",
			args:     []string{"app", "a", "t", "-v", "hi"},
			wantV:    true,
			wantArgs: []string{"hi"},
		},
		{
			name:     "both directions at once",
			args:     []string{"app", "-am", "some-model", "a", "t", "-v", "hi"},
			wantAm:   "some-model",
			wantV:    true,
			wantArgs: []string{"hi"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands, audioFs, transcribeFs := forwardTree()
			command, err := dispatch(tt.args, commands)
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if command.Describe() != "transcribe" {
				t.Fatalf("command: got %q want transcribe", command.Describe())
			}
			if got := transcribeFs.Lookup("am").Value.String(); got != tt.wantAm {
				t.Fatalf("-am: got %q want %q", got, tt.wantAm)
			}
			if got := transcribeFs.Lookup("x").Value.String(); got != boolString(tt.wantX) {
				t.Fatalf("-x: got %q want %v", got, tt.wantX)
			}
			if got := audioFs.Lookup("v").Value.String(); got != boolString(tt.wantV) {
				t.Fatalf("-v: got %q want %v", got, tt.wantV)
			}
			if got := command.Flagset().Args(); !equalStrings(got, tt.wantArgs) {
				t.Fatalf("positionals: got %v want %v", got, tt.wantArgs)
			}
		})
	}
}

// Test_dispatch_forwardKeepsLastWins pins that forwarding does not outrank
// an explicit value at the owning level: the tokens are injected before the
// level's own args, so the later one wins as it would in one flagset.
func Test_dispatch_forwardKeepsLastWins(t *testing.T) {
	commands, _, transcribeFs := forwardTree()
	if _, err := dispatch([]string{"app", "-am", "first", "a", "t", "-am", "second"}, commands); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := transcribeFs.Lookup("am").Value.String(); got != "second" {
		t.Fatalf("-am: got %q want second", got)
	}
}

// Test_dispatch_unresolvedOwnerErrors pins the limit of the convenience: a
// flag whose owning command is never resolved must fail loudly, since
// silently accepting a flag that configures nothing is the bug the hint
// was written for.
func Test_dispatch_unresolvedOwnerErrors(t *testing.T) {
	for _, args := range [][]string{
		{"app", "-am", "some-model", "a", "h"},
		{"app", "-am", "some-model", "q", "hi"},
		{"app", "-cm", "gpt-4", "a", "t"},
	} {
		commands, _, _ := forwardTree()
		_, err := dispatch(args, commands)
		var misplaced MisplacedFlagError
		if !errors.As(err, &misplaced) {
			t.Fatalf("%v: expected MisplacedFlagError, got %v", args, err)
		}
		if !strings.Contains(err.Error(), "belongs to") {
			t.Fatalf("%v: expected an owner hint, got %q", args, err.Error())
		}
	}
}

// Test_dispatch_unknownFlagStaysPlain pins that a flag no command defines
// keeps the stdlib error at every level: nothing is forwarded on a guess.
func Test_dispatch_unknownFlagStaysPlain(t *testing.T) {
	for _, args := range [][]string{
		{"app", "a", "t", "-nope"},
		{"app", "-nope", "a", "t"},
	} {
		commands, _, _ := forwardTree()
		_, err := dispatch(args, commands)
		var misplaced MisplacedFlagError
		if errors.As(err, &misplaced) {
			t.Fatalf("%v: expected no hint, got %v", args, err)
		}
		if err == nil || !strings.Contains(err.Error(), "not defined") {
			t.Fatalf("%v: expected the stdlib message, got %v", args, err)
		}
	}
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Test_dispatch_forwardedValueMustParse pins that forwarding does not
// loosen validation: a value the owning flag rejects fails as it would at
// its own level.
func Test_dispatch_forwardedValueMustParse(t *testing.T) {
	commands, _, _ := forwardTree()
	_, err := dispatch([]string{"app", "a", "t", "-par", "many"}, commands)
	if err == nil || !strings.Contains(err.Error(), "failed to set 'par'") {
		t.Fatalf("expected the owning flagset to reject the value, got %v", err)
	}
}

// Test_dispatch_forwardsThroughNestedLevels pins that the owner search
// descends the whole subtree, not just the next level.
func Test_dispatch_forwardsThroughNestedLevels(t *testing.T) {
	leafFs := flag.NewFlagSet("leaf", flag.ContinueOnError)
	leafFs.String("deep", "", "deep flag")
	middle := &mockSubcommander{
		mockCommand: mockCommand{
			describeFunc: func() string { return "middle" },
			flagSet:      flag.NewFlagSet("middle", flag.ContinueOnError),
		},
		subs: map[string]Command{"leaf|l": &mockCommand{
			describeFunc: func() string { return "leaf" },
			flagSet:      leafFs,
		}},
	}
	top := &mockSubcommander{
		mockCommand: mockCommand{
			describeFunc: func() string { return "top" },
			flagSet:      flag.NewFlagSet("top", flag.ContinueOnError),
		},
		subs: map[string]Command{"middle|m": middle},
	}

	command, err := dispatch([]string{"app", "-deep", "value", "top", "m", "l", "rest"}, map[string]Command{"top": top})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if command.Describe() != "leaf" {
		t.Fatalf("command: got %q want leaf", command.Describe())
	}
	if got := leafFs.Lookup("deep").Value.String(); got != "value" {
		t.Fatalf("-deep: got %q want value", got)
	}
	if got := command.Flagset().Args(); !equalStrings(got, []string{"rest"}) {
		t.Fatalf("positionals: got %v", got)
	}
}

// Test_takeFlag_stopsAtTerminator pins that nothing past "--" is treated as
// a flag to move.
func Test_takeFlag_stopsAtTerminator(t *testing.T) {
	args := []string{"--", "-am", "x"}
	if _, tokens, took := takeFlag(args, "am", true); took {
		t.Fatalf("expected no take past the terminator, got %v", tokens)
	}
	if _, _, took := takeFlag([]string{"hi"}, "am", true); took {
		t.Fatal("expected no take when the flag is absent")
	}
}
