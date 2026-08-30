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
// value-taking in any registered flagset, at any nesting level. If two
// commands define the same flag name with different arity, the scan treats
// it as value-taking. Each nesting level owns its own flag namespace, but
// placement is a convenience rather than a rule: a flag written at the
// wrong level is forwarded to the level that defines it, as long as that
// level is on the resolved path. One written for a command this run never
// reaches is an error naming its owner (MisplacedFlagError), since a flag
// that configures nothing must not pass silently.
func Run(ctx context.Context, args []string, commands map[string]Command, usage string) int {
	commands = withCompletion(binaryName(args), commands)
	if len(args) > 1 && args[1] == "__complete" && matchCommand("__complete", commands) == nil {
		return runComplete(args[2:], commands)
	}
	printUsage := func() {
		ancli.Okf("%v", getUsage(usage, commands))
	}
	command, err := dispatch(args, commands)
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

// dispatch resolves args to the command that will run: the top-level scan,
// then the Subcommander descent, with flags forwarded to the level that
// defines them (see forwardMisplaced).
func dispatch(args []string, commands map[string]Command) (Command, error) {
	command, pending, err := parseForward(args, commands)
	if err != nil {
		return command, err
	}
	return resolveSubcommands(command, commands, pending)
}

// parse resolves the top-level command and parses its flagset.
func parse(args []string, commands map[string]Command) (Command, error) {
	command, _, err := parseForward(args, commands)
	return command, err
}

func parseForward(args []string, commands map[string]Command) (Command, []pendingFlag, error) {
	if len(args) <= 1 {
		return nil, nil, ErrNoArgs
	}
	// Copy to avoid mutating the caller's backing array.
	args = slices.Clone(args)
	// Strip binary from args to find first argument
	args = args[1:]
	valueFlags, err := valueFlagUnion(commands)
	if err != nil {
		return nil, nil, err
	}
	cmdCandidate, cmdIdx := findCommandCandidate(args, valueFlags)
	if cmdIdx == -1 {
		return nil, nil, ErrNoArgs
	}
	command := matchCommand(cmdCandidate, commands)
	if command == nil {
		return nil, nil, unmatchedCommandError(args, cmdIdx, cmdCandidate, commands)
	}

	// Strip found command
	args = append(args[:cmdIdx], args[cmdIdx+1:]...)

	pending, err := parseLevel(command, args, nil, commands)
	if err != nil {
		return command, nil, err
	}
	return command, pending, nil
}

// pendingFlag is a flag the level being parsed does not define but one of
// its subcommands does: held aside until that level is resolved, or
// reported as misplaced if it never is.
type pendingFlag struct {
	name   string
	tokens []string
}

// parseLevel parses one level's flagset, forwarding any flag the level does
// not define to the level that does. A flag owned by an already-resolved
// ancestor is set there directly; one owned deeper is held pending until
// its level is resolved. Anything else keeps the plain parse error (with an
// owner hint when some off-path command defines it).
func parseLevel(command Command, args []string, ancestors []Command, commands map[string]Command) ([]pendingFlag, error) {
	var pending []pendingFlag
	for {
		err := parseFlagset(command.Flagset(), args)
		if err == nil {
			return pending, nil
		}
		name, isUndefined := undefinedFlagName(err)
		if !isUndefined {
			return nil, err
		}
		if owner := ancestorFlag(ancestors, name); owner != nil {
			rest, tokens, took := takeFlag(args, name, !isBoolFlag(owner))
			if !took {
				return nil, hintUndefinedFlag(err, commands)
			}
			if setErr := applyFlag(ancestorFlagset(ancestors, name), name, tokens); setErr != nil {
				return nil, setErr
			}
			args = rest
			continue
		}
		if owner := subtreeFlag(command, name); owner != nil {
			rest, tokens, took := takeFlag(args, name, !isBoolFlag(owner))
			if !took {
				return nil, hintUndefinedFlag(err, commands)
			}
			pending = append(pending, pendingFlag{name: name, tokens: tokens})
			args = rest
			continue
		}
		return nil, hintUndefinedFlag(err, commands)
	}
}

// undefinedFlagName reads the flag name out of stdlib's "flag provided but
// not defined" error, reporting false for every other parse failure.
func undefinedFlagName(err error) (string, bool) {
	const marker = "flag provided but not defined: "
	_, after, found := strings.Cut(err.Error(), marker)
	if !found {
		return "", false
	}
	return flagName(strings.TrimSpace(after))
}

// ancestorFlag finds a flag definition on an already-resolved ancestor,
// innermost first.
func ancestorFlag(ancestors []Command, name string) *flag.Flag {
	if fs := ancestorFlagset(ancestors, name); fs != nil {
		return fs.Lookup(name)
	}
	return nil
}

func ancestorFlagset(ancestors []Command, name string) *flag.FlagSet {
	for i := len(ancestors) - 1; i >= 0; i-- {
		fs := ancestors[i].Flagset()
		if fs != nil && fs.Lookup(name) != nil {
			return fs
		}
	}
	return nil
}

// subtreeFlag finds a flag definition below command, breadth-first, so the
// shallowest owner wins.
func subtreeFlag(command Command, name string) *flag.Flag {
	sc, isParent := command.(Subcommander)
	if !isParent {
		return nil
	}
	level := slices.Sorted(maps.Keys(sc.Subcommands()))
	for _, key := range level {
		sub := sc.Subcommands()[key]
		if fs := sub.Flagset(); fs != nil {
			if f := fs.Lookup(name); f != nil {
				return f
			}
		}
	}
	for _, key := range level {
		if f := subtreeFlag(sc.Subcommands()[key], name); f != nil {
			return f
		}
	}
	return nil
}

// takeFlag removes the first occurrence of the named flag from args,
// returning the remaining args and the removed tokens. Scanning stops at
// "--", past which nothing is a flag.
func takeFlag(args []string, name string, valued bool) ([]string, []string, bool) {
	for idx, arg := range args {
		if arg == "--" {
			return args, nil, false
		}
		written, isFlag := flagName(arg)
		if isFlag && written == name {
			end := idx + 1
			if valued && end < len(args) {
				end++
			}
			return slices.Concat(args[:idx:idx], args[end:]), slices.Clone(args[idx:end]), true
		}
		if strings.HasPrefix(arg, "-") && strings.Contains(arg, "=") {
			if eqName, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "="); eqName == name {
				return slices.Concat(args[:idx:idx], args[idx+1:]), []string{arg}, true
			}
		}
	}
	return args, nil, false
}

// applyFlag sets a forwarded flag on the flagset that owns it, without
// disturbing that flagset's parsed positionals.
func applyFlag(fs *flag.FlagSet, name string, tokens []string) error {
	value := "true"
	switch {
	case len(tokens) > 1:
		value = tokens[1]
	case strings.Contains(tokens[0], "="):
		_, value, _ = strings.Cut(tokens[0], "=")
	}
	if err := fs.Set(name, value); err != nil {
		return fmt.Errorf("failed to set '%v': %w", name, err)
	}
	return nil
}

// injectPending prepends the tokens of every pending flag this level
// defines to its args, returning the args and the flags still pending.
func injectPending(fs *flag.FlagSet, pending []pendingFlag, args []string) ([]string, []pendingFlag) {
	var forwarded []string
	remaining := pending[:0:0]
	for _, p := range pending {
		if fs.Lookup(p.name) != nil {
			forwarded = append(forwarded, p.tokens...)
			continue
		}
		remaining = append(remaining, p)
	}
	return slices.Concat(forwarded, args), remaining
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
// level's flagset with the args remaining after its parent's parse plus any
// flag forwarded to it. A flag still pending once the leaf is resolved was
// written for a command this run never reaches, which is an error: a flag
// that configures nothing must not pass silently.
func resolveSubcommands(command Command, commands map[string]Command, pending []pendingFlag) (Command, error) {
	ancestors := []Command{command}
	for {
		sc, ok := command.(Subcommander)
		if !ok {
			break
		}
		rest := command.Flagset().Args()
		if len(rest) == 0 {
			break
		}
		sub := matchCommand(rest[0], sc.Subcommands())
		if sub == nil {
			break
		}
		fs := sub.Flagset()
		if fs == nil {
			return sub, fmt.Errorf("flagset is nil, please define flagset")
		}
		args, stillPending := injectPending(fs, pending, rest[1:])
		found, err := parseLevel(sub, args, ancestors, commands)
		if err != nil {
			return sub, err
		}
		pending = append(stillPending, found...)
		ancestors = append(ancestors, sub)
		command = sub
	}
	if len(pending) > 0 {
		return command, pendingFlagError(pending[0], commands)
	}
	return command, nil
}

// pendingFlagError reports a forwarded flag whose owning command was never
// resolved, naming where it does belong.
func pendingFlagError(pending pendingFlag, commands map[string]Command) error {
	written := pending.tokens[0]
	return MisplacedFlagError{
		Flag:   written,
		Owners: flagOwners(commands)[pending.name],
		err:    fmt.Errorf("flag provided but not defined here: %v", written),
	}
}

// valueFlagUnion collects every value-taking (non-bool) flag name across
// every registered command and its Subcommander descendants, for
// arity-aware scanning: a sub-level flag written before its command must
// still consume its value, or that value is read as the command name.
func valueFlagUnion(commands map[string]Command) (map[string]bool, error) {
	valueFlags := map[string]bool{}
	var walk func(cmds map[string]Command) error
	walk = func(cmds map[string]Command) error {
		for _, c := range cmds {
			fs := c.Flagset()
			if fs == nil {
				return fmt.Errorf("flagset is nil, please define flagset")
			}
			fs.VisitAll(func(f *flag.Flag) {
				if !isBoolFlag(f) {
					valueFlags[f.Name] = true
				}
			})
			if sc, ok := c.(Subcommander); ok {
				if err := walk(sc.Subcommands()); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(commands); err != nil {
		return nil, err
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
