package cmd

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/baalimago/go_away_boilerplate/pkg/ancli"
	"github.com/baalimago/go_away_boilerplate/pkg/table"
)

// Run dispatches to the command named by the first positional token in args
// (args[0] must be the binary name). The scan preceding dispatch classifies
// tokens with stdlib flag semantics: "--" ends flag parsing (the next token
// is the command), a bare "-" is positional, "-name=value" consumes one
// token, and "-name" consumes the following token too when name is
// value-taking in any registered command's flagset. If two commands define
// the same flag name with different arity, the scan treats it as
// value-taking. Only top-level flagsets feed the scan; subcommand flags must
// appear after their subcommand — each nesting level is an independent flag
// namespace.
func Run(ctx context.Context, args []string, commands map[string]Command, usage string) int {
	commands = withCompletion(binaryName(args), commands)
	if len(args) > 1 && args[1] == "__complete" && matchCommand("__complete", commands) == nil {
		return runComplete(args[2:], commands)
	}
	printUsage := func() {
		ancli.Okf("%v", getUsage(usage, commands))
	}
	command, err := parse(args, commands)
	if err != nil {
		return printHelp(command, err, printUsage)
	}

	command, err = resolveSubcommands(command, commands)
	if err != nil {
		return printHelp(command, err, printUsage)
	}

	err = command.Setup(ctx)
	if err != nil {
		if isUserInitiatedExit(err) {
			return 0
		}
		ancli.Errf("failed to setup command: %v", err.Error())
		return 1
	}

	err = command.Run(ctx)
	if err != nil {
		if isUserInitiatedExit(err) {
			return 0
		}
		ancli.Errf("failed to run: %v", err.Error())
		return 1
	}
	return 0
}

func isUserInitiatedExit(err error) bool {
	return errors.Is(err, ErrUserInitiatedExit) || errors.Is(err, table.ErrUserInitiatedExit)
}

func parse(args []string, commands map[string]Command) (Command, error) {
	if len(args) <= 1 {
		return nil, ErrNoArgs
	}
	// Copy to avoid mutating the caller's backing array.
	args = slices.Clone(args)
	// Strip binary from args to find first argument
	args = args[1:]
	valueFlags, err := valueFlagUnion(commands)
	if err != nil {
		return nil, err
	}
	cmdCandidate, cmdIdx := findCommandCandidate(args, valueFlags)
	if cmdIdx == -1 {
		return nil, ErrNoArgs
	}
	command := matchCommand(cmdCandidate, commands)
	if command == nil {
		return nil, unmatchedCommandError(args, cmdIdx, cmdCandidate, commands)
	}

	// Strip found command
	args = append(args[:cmdIdx], args[cmdIdx+1:]...)

	err = parseFlagset(command.Flagset(), args)
	if err != nil {
		return command, hintUndefinedFlag(err, commands)
	}

	return command, nil
}

// unmatchedCommandError explains the common per-level flag mistake: when the
// token before the unmatched candidate is a flag this level does not define,
// the candidate is that flag's value rather than a command name.
func unmatchedCommandError(args []string, cmdIdx int, candidate string, commands map[string]Command) error {
	if cmdIdx == 0 {
		return ArgNotFoundError(candidate)
	}
	name, ok := flagName(args[cmdIdx-1])
	if !ok {
		return ArgNotFoundError(candidate)
	}
	owners, defined := flagOwners(commands)[name]
	if !defined {
		return ArgNotFoundError(candidate)
	}
	return MisplacedFlagError{
		Flag:      args[cmdIdx-1],
		Owners:    owners,
		Candidate: candidate,
		err:       ArgNotFoundError(candidate),
	}
}

// hintUndefinedFlag appends the owning command to stdlib's "flag provided
// but not defined" error when some other command defines that flag.
func hintUndefinedFlag(err error, commands map[string]Command) error {
	const marker = "flag provided but not defined: "
	msg := err.Error()
	_, after, ok := strings.Cut(msg, marker)
	if !ok {
		return err
	}
	written := strings.TrimSpace(after)
	name, ok := flagName(written)
	if !ok {
		return err
	}
	owners, defined := flagOwners(commands)[name]
	if !defined {
		return err
	}
	return MisplacedFlagError{Flag: written, Owners: owners, err: err}
}

// flagName strips the leading dashes of a flag token, reporting false for
// anything that is not a flag: positionals, a bare "-", "--", and the
// "-name=value" form (whose value never becomes a command candidate).
func flagName(token string) (string, bool) {
	if !strings.HasPrefix(token, "-") {
		return "", false
	}
	name := strings.TrimLeft(token, "-")
	if name == "" || strings.Contains(name, "=") {
		return "", false
	}
	return name, true
}

// flagOwners maps every flag name to the command paths defining it,
// descending through Subcommander trees. It runs on error paths only, so
// the walk costs nothing in the common case; Flagset() purity makes it safe.
func flagOwners(commands map[string]Command) map[string][]string {
	owners := map[string][]string{}
	var walk func(prefix string, cmds map[string]Command)
	walk = func(prefix string, cmds map[string]Command) {
		for nameWithShortcut, command := range cmds {
			name, _, _ := strings.Cut(nameWithShortcut, "|")
			path := strings.TrimSpace(prefix + " " + name)
			if fs := command.Flagset(); fs != nil {
				fs.VisitAll(func(f *flag.Flag) {
					owners[f.Name] = append(owners[f.Name], path)
				})
			}
			if sc, ok := command.(Subcommander); ok {
				walk(path, sc.Subcommands())
			}
		}
	}
	walk("", commands)
	for name := range owners {
		slices.Sort(owners[name])
		owners[name] = slices.Compact(owners[name])
	}
	return owners
}

// parseFlagset parses with the flagset's own output silenced: the
// dispatcher owns user-facing errors and help, so stdlib's "Usage of x:"
// dump and error echo would only duplicate them.
func parseFlagset(fs *flag.FlagSet, args []string) error {
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("failed to parse flagset: %w", err)
	}
	return nil
}

func matchCommand(candidate string, commands map[string]Command) Command {
	for nameWithShortcut, command := range commands {
		for name := range strings.SplitSeq(nameWithShortcut, "|") {
			if name == candidate {
				return command
			}
		}
	}
	return nil
}

// resolveSubcommands descends through Subcommander parents as long as the
// current command's first positional matches a subcommand key, parsing each
// level's flagset with the args remaining after its parent's parse.
func resolveSubcommands(command Command, commands map[string]Command) (Command, error) {
	for {
		sc, ok := command.(Subcommander)
		if !ok {
			return command, nil
		}
		rest := command.Flagset().Args()
		if len(rest) == 0 {
			return command, nil
		}
		sub := matchCommand(rest[0], sc.Subcommands())
		if sub == nil {
			return command, nil
		}
		fs := sub.Flagset()
		if fs == nil {
			return sub, fmt.Errorf("flagset is nil, please define flagset")
		}
		if err := parseFlagset(fs, rest[1:]); err != nil {
			return sub, hintUndefinedFlag(err, commands)
		}
		command = sub
	}
}

// valueFlagUnion collects every value-taking (non-bool) flag name across all
// registered commands, for arity-aware scanning.
func valueFlagUnion(commands map[string]Command) (map[string]bool, error) {
	valueFlags := map[string]bool{}
	for _, c := range commands {
		fs := c.Flagset()
		if fs == nil {
			return nil, fmt.Errorf("flagset is nil, please define flagset")
		}
		fs.VisitAll(func(f *flag.Flag) {
			if !isBoolFlag(f) {
				valueFlags[f.Name] = true
			}
		})
	}
	return valueFlags, nil
}

func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// findCommandCandidate returns the first positional token and its index, or
// ("", -1) when every token is consumed as a flag or flag value.
func findCommandCandidate(args []string, valueFlags map[string]bool) (string, int) {
	flagsEnded := false
	for idx := 0; idx < len(args); idx++ {
		arg := args[idx]
		if !flagsEnded && arg != "-" && strings.HasPrefix(arg, "-") {
			if arg == "--" {
				flagsEnded = true
				continue
			}
			name := strings.TrimLeft(arg, "-")
			if !strings.Contains(name, "=") && valueFlags[name] {
				idx++ // skip the flag's value
			}
			continue
		}
		return arg, idx
	}
	return "", -1
}

func printHelp(command Command, err error, printUsage UsagePrinter) int {
	var notValidArg ArgNotFoundError
	var misplacedFlag MisplacedFlagError
	if errors.As(err, &misplacedFlag) {
		ancli.Errf("%v", err.Error())
	} else if errors.As(err, &notValidArg) {
		ancli.Errf("%v", err.Error())
	} else if errors.Is(err, ErrNoArgs) {
	} else if errors.Is(err, flag.ErrHelp) && command != nil {
		ancli.Noticef("[command help]: %v", helpText(command))
		return 0
	} else {
		ancli.Errf("unknown error: %v", err.Error())
	}
	printUsage()
	return 1
}

func formatCommandDescriptions(commands map[string]Command) string {
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	for _, name := range slices.Sorted(maps.Keys(commands)) {
		fmt.Fprintf(w, "\t%v\t%v\n", name, commands[name].Describe())
	}
	w.Flush()
	return buf.String()
}

// Lookup resolves a command by name or alias, descending Subcommander
// trees for each subsequent path element. Nil when the path matches
// nothing.
func Lookup(commands map[string]Command, path ...string) Command {
	var command Command
	level := commands
	for _, name := range path {
		if level == nil {
			return nil
		}
		command = matchCommand(name, level)
		if command == nil {
			return nil
		}
		if sc, ok := command.(Subcommander); ok {
			level = sc.Subcommands()
		} else {
			level = nil
		}
	}
	return command
}

// HelpText renders a command's Help(), appending a Subcommander's
// subcommand table — the same composition the dispatcher prints on -h.
func HelpText(command Command) string {
	return helpText(command)
}

// helpText appends a Subcommander's subcommand table to its own Help().
func helpText(command Command) string {
	help := command.Help()
	if sc, ok := command.(Subcommander); ok {
		if subs := sc.Subcommands(); len(subs) > 0 {
			help += "\n\nsubcommands:\n" + DescribeSubcommands(subs)
		}
	}
	return help
}

// DescribeSubcommands renders a sorted name/description table of commands,
// for apps composing custom help layouts.
func DescribeSubcommands(subs map[string]Command) string {
	return formatCommandDescriptions(subs)
}

func getUsage(usage string, cmds map[string]Command) string {
	return fmt.Sprintf(usage, formatCommandDescriptions(cmds))
}
