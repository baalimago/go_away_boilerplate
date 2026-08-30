package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
)

type Command interface {
	Setup(context.Context) error

	// Run and block until context cancel
	Run(context.Context) error

	// Help by printing a usage string. Currently not used anywhere.
	Help() string

	// Describe the command shortly
	Describe() string

	// Flagset which defines the flags for the command. Must be pure and
	// memoized: no IO or side effects, and repeated calls return the same
	// *flag.FlagSet instance. The dispatcher and completion engine walk
	// every registered command's flagset on each invocation, and Parse
	// runs on the returned instance.
	Flagset() *flag.FlagSet
}

// Subcommander is optionally implemented by a Command that owns nested
// subcommands. Keys use the same "name|shortcut" form as the top-level
// command map. Subcommands() must be pure and memoized, like Flagset().
//
// Dispatch descends when the parent flagset's first positional matches a
// subcommand key; Setup and Run fire only on the executed leaf, never on a
// parent whose subcommand runs. An unmatched (or absent) first positional
// leaves all args with the parent's own Setup/Run. Each level is an
// independent flag namespace, so a name or abbreviation may be reused with
// different meaning or arity at different levels, but placement is
// forgiving: a flag written at another level of the resolved path reaches
// the level that defines it, so "app chat list -r", "app chat -r list" and
// "app -r chat list" are equivalent. Where a name is defined at several
// levels of one path, the shallowest owner takes it. A flag whose owner is
// not on the resolved path fails with MisplacedFlagError. A nil or empty
// map means no subcommands.
type Subcommander interface {
	Subcommands() map[string]Command
}

type (
	ArgParser    func([]string, map[string]Command) (Command, func(string, map[string]Command) string, error)
	UsagePrinter func()
)

type ArgNotFoundError string

func (e ArgNotFoundError) Error() string {
	return fmt.Sprintf("'%v' is not a valid argument\n", string(e))
}

// MisplacedFlagError reports a flag whose owning command is not on the
// resolved path, naming the command(s) that do define it. Flags written at
// another level of the path are forwarded rather than reported (see
// Subcommander); this is what remains: a flag that would configure a
// command this run never reaches.
type MisplacedFlagError struct {
	// Flag as written by the user, dashes included.
	Flag string
	// Owners are the command paths defining it, e.g. "audio transcribe".
	Owners []string
	// Candidate is the token the scan mistook for a command; empty when a
	// command's own flagset rejected the flag instead.
	Candidate string

	err error
}

func (e MisplacedFlagError) Error() string {
	if e.Candidate != "" {
		return fmt.Sprintf("'%v' is not a valid argument: it was read as the command name because '%v' is not a flag at this level.\nHint: '%v' belongs to %v — place it there, after that command.\n",
			e.Candidate, e.Flag, e.Flag, quotedOwners(e.Owners))
	}
	return fmt.Sprintf("%v\nHint: '%v' belongs to %v — place it there, after that command.\n", e.err, e.Flag, quotedOwners(e.Owners))
}

// quotedOwners renders the owning command paths as a readable list:
// "'audio transcribe'", "'chat' or 'query'", "'a', 'b' or 'c'".
func quotedOwners(owners []string) string {
	quoted := make([]string, 0, len(owners))
	for _, owner := range owners {
		quoted = append(quoted, "'"+owner+"'")
	}
	switch len(quoted) {
	case 0:
		return "another command"
	case 1:
		return quoted[0]
	default:
		return strings.Join(quoted[:len(quoted)-1], ", ") + " or " + quoted[len(quoted)-1]
	}
}

func (e MisplacedFlagError) Unwrap() error { return e.err }

var ErrNoArgs = errors.New("no arguments found")

// ErrUserInitiatedExit signals that the user deliberately ended the
// command (quit key, ctrl-c, "no thanks"). Run treats it as a clean,
// silent, successful exit.
var ErrUserInitiatedExit = errors.New("user initiated exit")
