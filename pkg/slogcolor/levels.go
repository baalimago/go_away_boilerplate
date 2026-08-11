package slogcolor

import (
	"fmt"
	"log/slog"
	"maps"
	"strings"
)

// DefaultModuleKey is the attribute key a record carries to select its
// module level. Bind it once per component:
//
//	logger := base.With(slogcolor.DefaultModuleKey, "mcp")
const DefaultModuleKey = "module"

// Levels is a default level plus per-module overrides. The zero value is
// unusable; build one with ParseLevels or NewLevels.
type Levels struct {
	def     slog.Level
	modules map[string]slog.Level
}

// NewLevels returns levels with the given default and no overrides.
func NewLevels(def slog.Level) *Levels {
	return &Levels{def: def, modules: map[string]slog.Level{}}
}

// Set overrides the level of one module.
func (l *Levels) Set(module string, level slog.Level) *Levels {
	l.modules[module] = level
	return l
}

// Default returns the level of a record with no module.
func (l *Levels) Default() slog.Level {
	if l == nil {
		return slog.LevelInfo
	}
	return l.def
}

// For returns the level of module, falling back to the default.
func (l *Levels) For(module string) slog.Level {
	if l == nil {
		return slog.LevelInfo
	}
	if level, ok := l.modules[module]; ok {
		return level
	}
	return l.def
}

// Modules returns the configured module overrides.
func (l *Levels) Modules() map[string]slog.Level {
	out := make(map[string]slog.Level, len(l.modules))
	maps.Copy(out, l.modules)
	return out
}

// String renders the levels in the ParseLevels grammar. Module entries are
// not ordered; use it for diagnostics, not for round-tripping.
func (l *Levels) String() string {
	var b strings.Builder
	for module, level := range l.modules {
		fmt.Fprintf(&b, "%s=%s,", module, strings.ToLower(level.String()))
	}
	b.WriteString(strings.ToLower(l.Default().String()))
	return b.String()
}

// ParseLevels reads the per-module level grammar:
//
//	LOG_LEVEL="cli=warn,mcp=debug,info"
//
// Each comma-separated entry is either "module=level", which sets that
// module's level, or a bare "level", which sets the default for every
// module without an override. Names and levels are case-insensitive and
// surrounding spaces are ignored. An empty string yields the Info default.
//
// The recognized levels are debug, info, warn (or warning), and error.
func ParseLevels(raw string) (*Levels, error) {
	levels := NewLevels(slog.LevelInfo)
	defaultSet := false
	for entry := range strings.SplitSeq(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		module, value, isModule := strings.Cut(entry, "=")
		module, value = strings.TrimSpace(module), strings.TrimSpace(value)
		if !isModule {
			level, err := ParseLevel(module)
			if err != nil {
				return nil, fmt.Errorf("default level: %w", err)
			}
			if defaultSet {
				return nil, fmt.Errorf("more than one default level in %q", raw)
			}
			levels.def = level
			defaultSet = true
			continue
		}
		if module == "" {
			return nil, fmt.Errorf("entry %q has an empty module name", entry)
		}
		level, err := ParseLevel(value)
		if err != nil {
			return nil, fmt.Errorf("module %q: %w", module, err)
		}
		if _, dup := levels.modules[module]; dup {
			return nil, fmt.Errorf("module %q is set more than once in %q", module, raw)
		}
		levels.modules[module] = level
	}
	return levels, nil
}

// ParseLevel maps one level name to a slog.Level.
func ParseLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown level %q, want debug, info, warn, or error", raw)
	}
}
