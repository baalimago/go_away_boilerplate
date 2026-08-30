// Package cmd is yet another cli tool abstraction. It's intended to be
// simple and pragmatic. See tests for example usecase.
//
// # Commands
//
// An app registers a map of "name|shortcut" keys to Command implementations
// and calls Run(ctx, os.Args, commands, usage). Command.Flagset() must be
// pure and memoized: no IO or side effects, repeated calls return the same
// *flag.FlagSet instance — both dispatch and completion walk every
// registered flagset on each invocation, and Parse runs on the returned
// instance.
//
// # Argument scan
//
// Run finds the command via an arity-aware scan that follows stdlib flag
// semantics: flags may precede the command ("-cm gpt-4 query hi" resolves
// "query"), "--" ends flag parsing, a bare "-" is positional, and
// "-name=value" consumes one token. If two commands define the same flag
// name with different arity (bool vs value-taking), the scan treats it as
// value-taking. The scan covers every registered flagset, nested levels
// included.
//
// # Subcommands
//
// A Command additionally implementing Subcommander gets nested dispatch:
// the parent flagset's first positional selects the subcommand, each level
// parses its own flags, and Setup/Run fire only on the executed leaf. Help
// for a Subcommander automatically appends its subcommand table (see
// DescribeSubcommands). Each level owns its flag namespace, but a flag
// written at the wrong level is forwarded to the level on the resolved
// path that defines it; one whose owner is never resolved fails with
// MisplacedFlagError naming that owner.
//
// # Completion
//
// Run auto-registers a "completion <bash|zsh>" command printing a shell
// script, and handles the hidden "__complete <words...>" protocol whose
// suggestions derive from the registered commands, flagsets, and
// Subcommander trees. Apps plug in value sources via the optional
// FlagValueCompleter and ArgCompleter interfaces. App-defined "completion"
// or "__complete" keys override the built-ins. Completion never calls
// Setup or Run of any command.
//
// # Clean exits
//
// Returning ErrUserInitiatedExit (or table.ErrUserInitiatedExit), wrapped
// or not, from Setup or Run makes Run exit 0 silently — the canonical
// "user chose to stop" outcome.
package cmd
