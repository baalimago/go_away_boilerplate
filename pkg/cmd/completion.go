package cmd

import (
	"context"
	"flag"
	"fmt"
	"maps"
	"path/filepath"
	"sort"
	"strings"
)

// CompletionKind classifies a suggestion: "plain" values are inserted as-is,
// "file" and "dir" tell the shell script to fall back to native file or
// directory completion.
type CompletionKind string

const (
	CompletionKindPlain CompletionKind = "plain"
	CompletionKindFile  CompletionKind = "file"
	CompletionKindDir   CompletionKind = "dir"
)

// CompletionItem is one suggestion emitted by the completion protocol:
// "<binary> __complete <shell words...>" (words[0] is the binary name)
// prints one "value\tkind" line per item.
type CompletionItem struct {
	Value string
	Kind  CompletionKind
}

// FlagValueCompleter is optionally implemented by a Command to complete
// values for its own flags. flagName is without dashes. The hook filters by
// partial itself. Value completion for a flag typed before any command is
// resolved is not offered.
type FlagValueCompleter interface {
	CompleteFlagValue(flagName, partial string) []CompletionItem
}

// ArgCompleter is optionally implemented by a Command to complete its
// positional arguments. Returning an empty non-nil slice means "complete
// nothing" (suppresses defaults). On a Subcommander, arg completions are
// appended after its subcommand names.
type ArgCompleter interface {
	CompleteArgs(args []string, partial string) []CompletionItem
}

// NewCompletionCommand returns the built-in completion command Run
// auto-registers, for apps that render their own command listings.
func NewCompletionCommand(binName string) Command {
	return &completionCommand{binName: binName}
}

// withCompletion returns a clone of commands with the built-in completion
// command injected, unless the app defines the key itself.
func withCompletion(binName string, commands map[string]Command) map[string]Command {
	if matchCommand("completion", commands) != nil {
		return commands
	}
	cloned := make(map[string]Command, len(commands)+1)
	maps.Copy(cloned, commands)
	cloned["completion"] = &completionCommand{binName: binName}
	return cloned
}

func runComplete(words []string, commands map[string]Command) int {
	for _, item := range completeWords(words, commands) {
		fmt.Printf("%s\t%s\n", item.Value, item.Kind)
	}
	return 0
}

// completeWords derives suggestions from the registered commands, their
// flagsets and Subcommander trees, plus the two optional hook interfaces.
// It never calls Setup or Run of any command.
func completeWords(words []string, commands map[string]Command) []CompletionItem {
	if len(words) <= 1 {
		return nil
	}
	// The first word is the binary name, per protocol.
	words = words[1:]
	current := words[len(words)-1]
	resolved, positionals, pendingValueFlag := resolveCompletionPath(words[:len(words)-1], commands)

	switch {
	case pendingValueFlag != "":
		if resolved == nil {
			return nil
		}
		return safeHook(func() []CompletionItem {
			if fvc, ok := resolved.(FlagValueCompleter); ok {
				return fvc.CompleteFlagValue(pendingValueFlag, current)
			}
			return nil
		})
	case strings.HasPrefix(current, "-"):
		return filterPlain(current, flagNames(resolved, commands))
	case resolved == nil:
		return append(
			filterPlain(current, commandNames(commands)),
			filterPlain(current, flagNames(nil, commands))...)
	default:
		var items []CompletionItem
		if sc, ok := resolved.(Subcommander); ok && len(sc.Subcommands()) > 0 {
			items = filterPlain(current, commandNames(sc.Subcommands()))
		}
		if ac, ok := resolved.(ArgCompleter); ok {
			items = append(items, safeHook(func() []CompletionItem {
				return ac.CompleteArgs(positionals, current)
			})...)
		}
		return items
	}
}

// resolveCompletionPath walks the words before the cursor, descending
// through commands and Subcommander levels while skipping flags and their
// values. pendingValueFlag is set when the final word is a value-taking
// flag, meaning the cursor holds its value.
func resolveCompletionPath(pre []string, commands map[string]Command) (resolved Command, positionals []string, pendingValueFlag string) {
	valueFlags, err := valueFlagUnion(commands)
	if err != nil {
		valueFlags = map[string]bool{}
	}
	level := commands
	for i := 0; i < len(pre); i++ {
		w := pre[i]
		if w != "-" && strings.HasPrefix(w, "-") {
			name := strings.TrimLeft(w, "-")
			if strings.Contains(name, "=") || !isValueFlag(name, resolved, valueFlags) {
				continue
			}
			if i == len(pre)-1 {
				pendingValueFlag = name
			} else {
				i++ // skip the flag's value
			}
			continue
		}
		if level != nil {
			if c := matchCommand(w, level); c != nil {
				resolved = c
				positionals = nil
				if sc, ok := c.(Subcommander); ok {
					level = sc.Subcommands()
				} else {
					level = nil
				}
				continue
			}
		}
		if resolved != nil {
			positionals = append(positionals, w)
		}
	}
	return resolved, positionals, pendingValueFlag
}

// isValueFlag scopes arity to the resolved command's flagset once one is
// known, falling back to the top-level union before that.
func isValueFlag(name string, resolved Command, valueFlags map[string]bool) bool {
	if resolved == nil {
		return valueFlags[name]
	}
	f := resolved.Flagset().Lookup(name)
	return f != nil && !isBoolFlag(f)
}

func flagNames(resolved Command, commands map[string]Command) []string {
	var names []string
	collect := func(f *flag.Flag) {
		names = append(names, "-"+f.Name)
	}
	if resolved != nil {
		resolved.Flagset().VisitAll(collect)
		return names
	}
	seen := map[string]bool{}
	for _, c := range commands {
		if fs := c.Flagset(); fs != nil {
			fs.VisitAll(func(f *flag.Flag) {
				if !seen[f.Name] {
					seen[f.Name] = true
					collect(f)
				}
			})
		}
	}
	return names
}

func commandNames(commands map[string]Command) []string {
	var names []string
	for key := range commands {
		names = append(names, strings.Split(key, "|")...)
	}
	return names
}

func filterPlain(prefix string, options []string) []CompletionItem {
	var items []CompletionItem
	sort.Strings(options)
	for _, option := range options {
		if strings.HasPrefix(option, prefix) {
			items = append(items, CompletionItem{Value: option, Kind: CompletionKindPlain})
		}
	}
	return items
}

// safeHook shields the shell session from a panicking app hook: recovered
// panics yield no suggestions.
func safeHook(fn func() []CompletionItem) (items []CompletionItem) {
	defer func() {
		if recover() != nil {
			items = nil
		}
	}()
	return fn()
}

type completionCommand struct {
	binName string
	flagset *flag.FlagSet
}

func (c *completionCommand) Setup(context.Context) error { return nil }

func (c *completionCommand) Run(context.Context) error {
	rest := c.Flagset().Args()
	if len(rest) == 0 {
		return fmt.Errorf("usage: %v completion <bash|zsh>", c.binName)
	}
	switch rest[0] {
	case "bash":
		fmt.Print(bashCompletionScript(c.binName))
	case "zsh":
		fmt.Print(zshCompletionScript(c.binName))
	default:
		return fmt.Errorf("unsupported shell %q, usage: %v completion <bash|zsh>", rest[0], c.binName)
	}
	return nil
}

func (c *completionCommand) Help() string {
	return fmt.Sprintf("print a shell completion script; install with e.g. 'source <(%v completion bash)'", c.binName)
}

func (c *completionCommand) Describe() string {
	return "generate shell completion scripts (bash|zsh)"
}

// CompleteArgs offers the supported shell names for the first positional.
func (c *completionCommand) CompleteArgs(args []string, partial string) []CompletionItem {
	if len(args) > 0 {
		return []CompletionItem{}
	}
	return filterPlain(partial, []string{"bash", "zsh"})
}

func (c *completionCommand) Flagset() *flag.FlagSet {
	if c.flagset == nil {
		c.flagset = flag.NewFlagSet("completion", flag.ContinueOnError)
	}
	return c.flagset
}

func binaryName(args []string) string {
	if len(args) == 0 {
		return "app"
	}
	return filepath.Base(args[0])
}

func bashCompletionScript(binName string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
_%[1]s_completion() {
  local IFS=$'\n'
  COMPREPLY=()
  local out
  out=$(%[1]s __complete "${COMP_WORDS[@]}")
  local value kind
  while IFS=$'\t' read -r value kind; do
    [[ -z "$value" ]] && continue
    case "$kind" in
      file)
        COMPREPLY+=( $(compgen -f -- "${COMP_WORDS[COMP_CWORD]}") )
        ;;
      dir)
        COMPREPLY+=( $(compgen -d -- "${COMP_WORDS[COMP_CWORD]}") )
        ;;
      *)
        COMPREPLY+=( "$value" )
        ;;
    esac
  done <<< "$out"
}
complete -F _%[1]s_completion %[1]s
`, binName)
}

func zshCompletionScript(binName string) string {
	return fmt.Sprintf(`#compdef %[1]s
_%[1]s_completion() {
  local -a lines
  lines=("${(@f)$(%[1]s __complete "${words[@]}")}")
  local line value kind
  for line in "${lines[@]}"; do
    value="${line%%%%$'\t'*}"
    kind="${line#*$'\t'}"
    case "$kind" in
      file)
        _files
        return
        ;;
      dir)
        _files -/
        return
        ;;
      *)
        compadd -- "$value"
        ;;
    esac
  done
}
compdef _%[1]s_completion %[1]s
`, binName)
}
