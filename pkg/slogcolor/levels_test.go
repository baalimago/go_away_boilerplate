package slogcolor

import (
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevels(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		raw     string
		def     slog.Level
		modules map[string]slog.Level
	}{
		{name: "empty is info", raw: "", def: slog.LevelInfo},
		{name: "bare default", raw: "debug", def: slog.LevelDebug},
		{
			name: "documented example", raw: "cli=warn,mcp=debug,info",
			def:     slog.LevelInfo,
			modules: map[string]slog.Level{"cli": slog.LevelWarn, "mcp": slog.LevelDebug},
		},
		{
			name: "default may come first", raw: "error,cli=debug",
			def:     slog.LevelError,
			modules: map[string]slog.Level{"cli": slog.LevelDebug},
		},
		{
			name: "spaces and case are ignored", raw: " CLI = Warn , mcp=DEBUG , Info ",
			def:     slog.LevelInfo,
			modules: map[string]slog.Level{"CLI": slog.LevelWarn, "mcp": slog.LevelDebug},
		},
		{
			name: "warning is warn", raw: "cli=warning",
			def:     slog.LevelInfo,
			modules: map[string]slog.Level{"cli": slog.LevelWarn},
		},
		{
			name: "empty entries are skipped", raw: ",cli=warn,,",
			def:     slog.LevelInfo,
			modules: map[string]slog.Level{"cli": slog.LevelWarn},
		},
		{name: "modules only keeps the info default", raw: "cli=error", def: slog.LevelInfo, modules: map[string]slog.Level{"cli": slog.LevelError}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			levels, err := ParseLevels(tt.raw)
			if err != nil {
				t.Fatalf("ParseLevels(%q) = %v", tt.raw, err)
			}
			if got := levels.Default(); got != tt.def {
				t.Fatalf("default = %v, want %v", got, tt.def)
			}
			for module, want := range tt.modules {
				if got := levels.For(module); got != want {
					t.Fatalf("module %q = %v, want %v", module, got, want)
				}
			}
			if got := len(levels.Modules()); got != len(tt.modules) {
				t.Fatalf("modules = %v, want %v", levels.Modules(), tt.modules)
			}
			// An unconfigured module always falls back to the default.
			if got := levels.For("absent"); got != tt.def {
				t.Fatalf("unconfigured module = %v, want the default %v", got, tt.def)
			}
		})
	}
}

func TestParseLevelsRejectsBadInput(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, raw, want string
	}{
		{name: "unknown default", raw: "verbose", want: "unknown level"},
		{name: "unknown module level", raw: "cli=verbose", want: `module "cli"`},
		{name: "empty module name", raw: "=warn", want: "empty module name"},
		{name: "two defaults", raw: "info,debug", want: "more than one default"},
		{name: "duplicate module", raw: "cli=warn,cli=debug", want: "more than once"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseLevels(tt.raw)
			if err == nil {
				t.Fatalf("ParseLevels(%q) = nil, want an error", tt.raw)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParseLevels(%q) = %v, want it to mention %q", tt.raw, err, tt.want)
			}
		})
	}
}

// TestLevelsStringRoundTrips proves String renders the grammar ParseLevels
// accepts, so a diagnostic can be pasted back into LOG_LEVEL.
func TestLevelsStringRoundTrips(t *testing.T) {
	t.Parallel()
	levels, err := ParseLevels("cli=warn,mcp=debug,error")
	if err != nil {
		t.Fatalf("ParseLevels() = %v", err)
	}
	again, err := ParseLevels(levels.String())
	if err != nil {
		t.Fatalf("ParseLevels(%q) = %v", levels.String(), err)
	}
	if again.Default() != slog.LevelError ||
		again.For("cli") != slog.LevelWarn || again.For("mcp") != slog.LevelDebug {
		t.Fatalf("round trip = %q, want the original levels", again.String())
	}
}

// TestNilLevelsAreInfo proves the zero configuration never panics and never
// silences a process.
func TestNilLevelsAreInfo(t *testing.T) {
	t.Parallel()
	var levels *Levels
	if got := levels.Default(); got != slog.LevelInfo {
		t.Fatalf("nil default = %v, want Info", got)
	}
	if got := levels.For("cli"); got != slog.LevelInfo {
		t.Fatalf("nil module = %v, want Info", got)
	}
}
