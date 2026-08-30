package cmd

import (
	"context"
	"errors"
	"flag"
	"strings"
	"testing"

	"github.com/baalimago/go_away_boilerplate/pkg/testboil"
)

type mockSubcommander struct {
	mockCommand
	subs map[string]Command
}

func (m *mockSubcommander) Subcommands() map[string]Command { return m.subs }

// chatTree returns a fresh chat|c (Bool r; subs list|l with Bool x, del)
// tree plus recorders for which leaves ran.
type chatTree struct {
	commands   map[string]Command
	chatFs     *flag.FlagSet
	listFs     *flag.FlagSet
	chat       *mockSubcommander
	list, del  *mockCommand
	chatSetup  bool
	chatRan    bool
	listRan    bool
	chatArgs   []string
	listRunErr error
}

func newChatTree() *chatTree {
	tr := &chatTree{
		chatFs: flag.NewFlagSet("chat", flag.ContinueOnError),
		listFs: flag.NewFlagSet("list", flag.ContinueOnError),
	}
	tr.chatFs.Bool("r", false, "")
	tr.listFs.Bool("x", false, "")
	tr.list = &mockCommand{
		describeFunc: func() string { return "list chats" },
		helpFunc:     func() string { return "list help text" },
		setupFunc:    func() error { return nil },
		runFunc: func(context.Context) error {
			tr.listRan = true
			return tr.listRunErr
		},
		flagSet: tr.listFs,
	}
	tr.del = &mockCommand{
		describeFunc: func() string { return "delete chats" },
		setupFunc:    func() error { return nil },
		runFunc:      func(context.Context) error { return nil },
		flagSet:      flag.NewFlagSet("del", flag.ContinueOnError),
	}
	tr.chat = &mockSubcommander{
		mockCommand: mockCommand{
			describeFunc: func() string { return "manage chats" },
			helpFunc:     func() string { return "chat help text" },
			setupFunc: func() error {
				tr.chatSetup = true
				return nil
			},
			runFunc: func(context.Context) error {
				tr.chatRan = true
				tr.chatArgs = tr.chatFs.Args()
				return nil
			},
			flagSet: tr.chatFs,
		},
		subs: map[string]Command{"list|l": tr.list, "del": tr.del},
	}
	tr.commands = map[string]Command{
		"chat|c": tr.chat,
		"q|query": &mockCommand{
			describeFunc: func() string { return "query" },
			flagSet:      flag.NewFlagSet("query", flag.ContinueOnError),
		},
	}
	return tr
}

func runTree(t *testing.T, tr *chatTree, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	stderr = testboil.CaptureStderr(t, func(t *testing.T) {
		stdout = testboil.CaptureStdout(t, func(t *testing.T) {
			code = Run(context.Background(), append([]string{"bin"}, args...),
				tr.commands, "usage: %v")
		})
	})
	return code, stdout, stderr
}

func Test_Run_subcommanderDescent(t *testing.T) {
	t.Run("descend by full name, leaf-only Setup/Run", func(t *testing.T) {
		tr := newChatTree()
		code, _, _ := runTree(t, tr, "chat", "list")
		if code != 0 {
			t.Fatalf("Run() = %v, want 0", code)
		}
		if !tr.listRan {
			t.Fatal("expected list.Run to execute")
		}
		if tr.chatRan || tr.chatSetup {
			t.Fatalf("parent must not fire: chatRan=%v chatSetup=%v", tr.chatRan, tr.chatSetup)
		}
	})

	t.Run("descend by shortcut at both levels", func(t *testing.T) {
		tr := newChatTree()
		code, _, _ := runTree(t, tr, "c", "l")
		if code != 0 || !tr.listRan || tr.chatRan {
			t.Fatalf("code=%v listRan=%v chatRan=%v", code, tr.listRan, tr.chatRan)
		}
	})

	t.Run("parent flag before sub, sub flag after", func(t *testing.T) {
		tr := newChatTree()
		code, _, _ := runTree(t, tr, "chat", "-r", "list", "-x")
		if code != 0 || !tr.listRan {
			t.Fatalf("code=%v listRan=%v", code, tr.listRan)
		}
		if tr.chatFs.Lookup("r").Value.String() != "true" {
			t.Error("expected r==true on chat's flagset")
		}
		if tr.listFs.Lookup("x").Value.String() != "true" {
			t.Error("expected x==true on list's flagset")
		}
	})

	t.Run("unmatched positional stays with parent", func(t *testing.T) {
		tr := newChatTree()
		code, _, _ := runTree(t, tr, "chat", "banana")
		if code != 0 || !tr.chatRan || !tr.chatSetup {
			t.Fatalf("code=%v chatRan=%v chatSetup=%v", code, tr.chatRan, tr.chatSetup)
		}
		if len(tr.chatArgs) != 1 || tr.chatArgs[0] != "banana" {
			t.Fatalf("chat Args() = %v, want [banana]", tr.chatArgs)
		}
		if tr.listRan {
			t.Fatal("list must not run")
		}
	})

	t.Run("no positional runs parent", func(t *testing.T) {
		tr := newChatTree()
		code, _, _ := runTree(t, tr, "chat")
		if code != 0 || !tr.chatRan {
			t.Fatalf("code=%v chatRan=%v", code, tr.chatRan)
		}
	})

	t.Run("sub help routes to sub", func(t *testing.T) {
		tr := newChatTree()
		code, stdout, _ := runTree(t, tr, "chat", "list", "-h")
		if code != 0 {
			t.Fatalf("Run() = %v, want 0", code)
		}
		if !strings.Contains(stdout, "list help text") {
			t.Fatalf("expected sub help, got: %q", stdout)
		}
		if strings.Contains(stdout, "chat help text") {
			t.Fatalf("parent help must not print, got: %q", stdout)
		}
	})

	t.Run("parent help lists sorted sub table", func(t *testing.T) {
		tr := newChatTree()
		code, stdout, _ := runTree(t, tr, "chat", "-h")
		if code != 0 {
			t.Fatalf("Run() = %v, want 0", code)
		}
		if !strings.Contains(stdout, "chat help text") {
			t.Fatalf("expected parent help, got: %q", stdout)
		}
		delIdx := strings.Index(stdout, "del")
		listIdx := strings.Index(stdout, "list|l")
		if delIdx == -1 || listIdx == -1 || delIdx > listIdx {
			t.Fatalf("expected sorted sub table (del before list|l), got: %q", stdout)
		}
	})

	t.Run("sentinel at depth exits silently", func(t *testing.T) {
		tr := newChatTree()
		tr.listRunErr = ErrUserInitiatedExit
		code, stdout, stderr := runTree(t, tr, "chat", "list")
		if code != 0 {
			t.Fatalf("Run() = %v, want 0", code)
		}
		if stdout != "" || stderr != "" {
			t.Fatalf("expected silence, got stdout=%q stderr=%q", stdout, stderr)
		}
	})

	t.Run("two-level nesting reaches the leaf", func(t *testing.T) {
		leafRan := false
		leaf := &mockCommand{
			describeFunc: func() string { return "leaf" },
			setupFunc:    func() error { return nil },
			runFunc: func(context.Context) error {
				leafRan = true
				return nil
			},
			flagSet: flag.NewFlagSet("leaf", flag.ContinueOnError),
		}
		mid := &mockSubcommander{
			mockCommand: mockCommand{
				describeFunc: func() string { return "mid" },
				flagSet:      flag.NewFlagSet("mid", flag.ContinueOnError),
			},
			subs: map[string]Command{"leaf": leaf},
		}
		top := &mockSubcommander{
			mockCommand: mockCommand{
				describeFunc: func() string { return "top" },
				flagSet:      flag.NewFlagSet("top", flag.ContinueOnError),
			},
			subs: map[string]Command{"mid|m": mid},
		}
		code := Run(context.Background(), []string{"bin", "top", "m", "leaf"},
			map[string]Command{"top": top}, "usage: %v")
		if code != 0 || !leafRan {
			t.Fatalf("code=%v leafRan=%v", code, leafRan)
		}
	})
}

func Test_Lookup(t *testing.T) {
	tr := newChatTree()

	t.Run("resolves names, aliases and nested paths", func(t *testing.T) {
		for _, path := range [][]string{{"chat"}, {"c"}} {
			if got := Lookup(tr.commands, path...); got != Command(tr.chat) {
				t.Fatalf("Lookup(%v) = %v, want chat", path, got)
			}
		}
		for _, path := range [][]string{{"chat", "list"}, {"c", "l"}} {
			if got := Lookup(tr.commands, path...); got != Command(tr.list) {
				t.Fatalf("Lookup(%v) = %v, want list", path, got)
			}
		}
	})

	t.Run("nil on unknown or over-deep paths", func(t *testing.T) {
		for _, path := range [][]string{{}, {"bogus"}, {"chat", "bogus"}, {"chat", "list", "deeper"}, {"q", "sub"}} {
			if got := Lookup(tr.commands, path...); got != nil {
				t.Fatalf("Lookup(%v) = %v, want nil", path, got)
			}
		}
	})
}

func Test_HelpText(t *testing.T) {
	tr := newChatTree()
	got := HelpText(tr.commands["chat|c"])
	if !strings.Contains(got, "chat help text") || !strings.Contains(got, "list|l") {
		t.Fatalf("expected composed help with sub table, got: %q", got)
	}
	if got := HelpText(tr.list); strings.Contains(got, "subcommands:") {
		t.Fatalf("leaf help must not carry a sub table, got: %q", got)
	}
}

func Test_Run_subcommanderErrors(t *testing.T) {
	t.Run("sub flagset parse error", func(t *testing.T) {
		tr := newChatTree()
		code, _, stderr := runTree(t, tr, "chat", "list", "-bogus")
		if code != 1 {
			t.Fatalf("Run() = %v, want 1", code)
		}
		if !strings.Contains(stderr, "failed to parse flagset") {
			t.Fatalf("expected parse error, got: %q", stderr)
		}
	})

	t.Run("sub flagset nil", func(t *testing.T) {
		tr := newChatTree()
		tr.list.flagSet = nil
		code, _, stderr := runTree(t, tr, "chat", "list")
		if code != 1 {
			t.Fatalf("Run() = %v, want 1", code)
		}
		if !strings.Contains(stderr, "flagset is nil") {
			t.Fatalf("expected nil-flagset error, got: %q", stderr)
		}
	})

	t.Run("nil or empty Subcommands treated as non-Subcommander", func(t *testing.T) {
		for name, subs := range map[string]map[string]Command{"nil": nil, "empty": {}} {
			t.Run(name, func(t *testing.T) {
				tr := newChatTree()
				tr.chat.subs = subs
				code, _, _ := runTree(t, tr, "chat", "banana")
				if code != 0 || !tr.chatRan {
					t.Fatalf("code=%v chatRan=%v", code, tr.chatRan)
				}
			})
		}
	})

	t.Run("sub Setup failure leaves parent untouched", func(t *testing.T) {
		tr := newChatTree()
		tr.list.setupFunc = func() error { return errors.New("sub setup boom") }
		code, _, stderr := runTree(t, tr, "chat", "list")
		if code != 1 {
			t.Fatalf("Run() = %v, want 1", code)
		}
		if !strings.Contains(stderr, "failed to setup command") {
			t.Fatalf("expected setup error, got: %q", stderr)
		}
		if tr.chatSetup || tr.chatRan {
			t.Fatalf("parent must be untouched: setup=%v ran=%v", tr.chatSetup, tr.chatRan)
		}
	})
}
