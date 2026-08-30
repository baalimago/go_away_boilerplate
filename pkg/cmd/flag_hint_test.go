package cmd

import (
	"errors"
	"flag"
	"strings"
	"testing"
)

// hintTree returns a top-level "q|query" (value flag -cm) plus an
// "audio|a" subcommander whose "transcribe|t" sub owns the value flag -am
// and the bool flag -x — the shape that produced the confusing error: a
// sub-level value flag placed before the command swallowed nothing, so its
// value was read as the command name.
func hintTree() map[string]Command {
	qFs := flag.NewFlagSet("query", flag.ContinueOnError)
	qFs.String("cm", "", "chat model")
	transcribeFs := flag.NewFlagSet("transcribe", flag.ContinueOnError)
	transcribeFs.String("am", "", "audio model")
	transcribeFs.Bool("x", false, "bool sub flag")
	transcribe := &mockCommand{
		describeFunc: func() string { return "transcribe" },
		flagSet:      transcribeFs,
	}
	audio := &mockSubcommander{
		mockCommand: mockCommand{
			describeFunc: func() string { return "audio" },
			flagSet:      flag.NewFlagSet("audio", flag.ContinueOnError),
		},
		subs: map[string]Command{"transcribe|t": transcribe},
	}
	query := &mockCommand{
		describeFunc: func() string { return "query" },
		flagSet:      qFs,
	}
	// Keys follow the documented "name|shortcut" order, so hints name the
	// readable long form.
	return map[string]Command{"query|q": query, "audio|a": audio}
}

func Test_flagOwners(t *testing.T) {
	owners := flagOwners(hintTree())

	if got := owners["am"]; len(got) != 1 || got[0] != "audio transcribe" {
		t.Fatalf("am owners: got %v", got)
	}
	if got := owners["cm"]; len(got) != 1 || got[0] != "query" {
		t.Fatalf("cm owners: got %v", got)
	}
	if _, exists := owners["nope"]; exists {
		t.Fatal("unknown flag must have no owner")
	}
}

// Test_parse_misplacedSubLevelFlag pins the hint for the reported failure:
// a sub-level value flag before the command makes its value look like the
// command name, so the error must name the flag and where it belongs.
func Test_parse_misplacedSubLevelFlag(t *testing.T) {
	_, err := parse([]string{"app", "-am", "some-model", "q", "hi"}, hintTree())

	var misplaced MisplacedFlagError
	if !errors.As(err, &misplaced) {
		t.Fatalf("expected MisplacedFlagError, got %v", err)
	}
	if misplaced.Flag != "-am" {
		t.Fatalf("flag: got %q", misplaced.Flag)
	}
	if len(misplaced.Owners) != 1 || misplaced.Owners[0] != "audio transcribe" {
		t.Fatalf("owners: got %v", misplaced.Owners)
	}
	msg := err.Error()
	for _, want := range []string{"-am", "audio transcribe", "some-model"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q missing %q", msg, want)
		}
	}
}

// Test_parse_unknownCommandKeepsPlainError pins that the hint is not
// invented for a token that is simply not a command.
func Test_parse_unknownCommandKeepsPlainError(t *testing.T) {
	_, err := parse([]string{"app", "bogus"}, hintTree())

	var misplaced MisplacedFlagError
	if errors.As(err, &misplaced) {
		t.Fatalf("expected no hint, got %v", err)
	}
	var notFound ArgNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected ArgNotFoundError, got %v", err)
	}
}

// Test_parse_topLevelValueFlagStillScans guards the arity union: a known
// top-level value flag consumes its value, so no hint path is reached.
func Test_parse_topLevelValueFlagStillScans(t *testing.T) {
	command, err := parse([]string{"app", "-cm", "gpt-4", "q", "hi"}, hintTree())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if command.Describe() != "query" {
		t.Fatalf("command: got %q", command.Describe())
	}
}

// Test_parseFlagset_undefinedFlagHint pins the second error site: a flag
// placed on a command that does not own it reports where it does.
func Test_parseFlagset_undefinedFlagHint(t *testing.T) {
	_, err := parse([]string{"app", "q", "-am", "some-model"}, hintTree())
	if err == nil {
		t.Fatal("expected parse error")
	}

	var misplaced MisplacedFlagError
	if !errors.As(err, &misplaced) {
		t.Fatalf("expected MisplacedFlagError, got %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"-am", "audio transcribe"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q missing %q", msg, want)
		}
	}
}

// Test_parseFlagset_undefinedUnknownFlagStaysPlain pins that a flag no
// command defines keeps the stdlib error, without a fabricated hint.
func Test_parseFlagset_undefinedUnknownFlagStaysPlain(t *testing.T) {
	_, err := parse([]string{"app", "q", "-nope"}, hintTree())
	if err == nil {
		t.Fatal("expected parse error")
	}
	var misplaced MisplacedFlagError
	if errors.As(err, &misplaced) {
		t.Fatalf("expected no hint, got %v", err)
	}
	if !strings.Contains(err.Error(), "not defined") {
		t.Fatalf("expected the stdlib message, got %q", err.Error())
	}
}
