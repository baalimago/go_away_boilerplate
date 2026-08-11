package slogcolor

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// newTestLogger returns a logger over a plain handler and its buffer.
func newTestLogger(t *testing.T, opts *Options) (*slog.Logger, *bytes.Buffer, *Handler) {
	t.Helper()
	buf := &bytes.Buffer{}
	if opts == nil {
		opts = &Options{}
	}
	opts.NoColor = true
	h := New(buf, opts)
	return slog.New(h), buf, h
}

// TestBoundAttrsSurvive is the regression this package exists for: a handler
// whose WithAttrs drops bound attributes silently destroys every correlation
// id a caller binds once and relies on for the rest of a request.
func TestBoundAttrsSurvive(t *testing.T) {
	t.Parallel()
	log, buf, _ := newTestLogger(t, nil)
	log.With("reqID", "abc123", "tool", "notes_pull").Info("call started", "attempt", 1)

	out := buf.String()
	for _, want := range []string{"reqID=abc123", "tool=notes_pull", "attempt=1", `msg="call started"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("record = %q, want it to contain %q", out, want)
		}
	}
}

// TestWithAttrsDoesNotLeakBetweenSiblings proves a derived logger's bindings
// stay out of its sibling's records.
func TestWithAttrsDoesNotLeakBetweenSiblings(t *testing.T) {
	t.Parallel()
	log, buf, _ := newTestLogger(t, nil)
	base := log.With("shared", "yes")
	base.With("only", "a").Info("a")
	base.With("only", "b").Info("b")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2: %q", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], "only=a") || strings.Contains(lines[0], "only=b") {
		t.Fatalf("first record = %q, want only=a alone", lines[0])
	}
	if !strings.Contains(lines[1], "only=b") || strings.Contains(lines[1], "only=a") {
		t.Fatalf("second record = %q, want only=b alone", lines[1])
	}
	for _, line := range lines {
		if !strings.Contains(line, "shared=yes") {
			t.Fatalf("record = %q, want the shared binding", line)
		}
	}
}

// TestGroupsQualifyKeys proves WithGroup prefixes both bound and call-site
// attributes, matching slog.TextHandler.
func TestGroupsQualifyKeys(t *testing.T) {
	t.Parallel()
	log, buf, _ := newTestLogger(t, nil)
	log.WithGroup("req").With("id", "7").Info("hello", "stage", "scan")

	out := buf.String()
	for _, want := range []string{"req.id=7", "req.stage=scan"} {
		if !strings.Contains(out, want) {
			t.Fatalf("record = %q, want %q", out, want)
		}
	}
}

// TestGroupAttrIsFlattened proves an inline slog.Group renders as dotted
// keys rather than an opaque value.
func TestGroupAttrIsFlattened(t *testing.T) {
	t.Parallel()
	log, buf, _ := newTestLogger(t, nil)
	log.Info("hello", slog.Group("s3", "bucket", "notes", "gen", 4))

	out := buf.String()
	for _, want := range []string{"s3.bucket=notes", "s3.gen=4"} {
		if !strings.Contains(out, want) {
			t.Fatalf("record = %q, want %q", out, want)
		}
	}
}

// secret is a LogValuer whose real value is hidden behind LogValue.
type secret struct{ shown string }

func (s secret) LogValue() slog.Value { return slog.StringValue(s.shown) }

// TestLogValuerIsResolved proves values are resolved before rendering, so a
// LogValuer renders its intended text instead of its struct.
func TestLogValuerIsResolved(t *testing.T) {
	t.Parallel()
	log, buf, _ := newTestLogger(t, nil)
	log.With("cred", secret{shown: "[redacted]"}).Info("hi", "other", secret{shown: "also"})

	out := buf.String()
	for _, want := range []string{"cred=[redacted]", "other=also"} {
		if !strings.Contains(out, want) {
			t.Fatalf("record = %q, want %q", out, want)
		}
	}
}

// TestModuleLevels proves the documented grammar reaches the handler: each
// module filters at its own level and everything else takes the default.
func TestModuleLevels(t *testing.T) {
	t.Parallel()
	levels, err := ParseLevels("cli=warn,mcp=debug,info")
	if err != nil {
		t.Fatalf("ParseLevels() = %v", err)
	}
	log, buf, _ := newTestLogger(t, &Options{Levels: levels})

	cli := log.With(DefaultModuleKey, "cli")
	mcp := log.With(DefaultModuleKey, "mcp")
	other := log.With(DefaultModuleKey, "storage")

	cli.Info("cli info is below warn")
	cli.Warn("cli warn is emitted")
	mcp.Debug("mcp debug is emitted")
	other.Debug("storage debug is below info")
	other.Info("storage info is emitted")
	log.Debug("unmoduled debug is below info")
	log.Info("unmoduled info is emitted")

	out := buf.String()
	for _, want := range []string{
		"cli warn is emitted", "mcp debug is emitted",
		"storage info is emitted", "unmoduled info is emitted",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output = %q, want it to contain %q", out, want)
		}
	}
	for _, unwanted := range []string{
		"cli info is below warn", "storage debug is below info", "unmoduled debug is below info",
	} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("output = %q, must not contain %q", out, unwanted)
		}
	}
}

// TestModuleAttrIsStillRendered proves selecting a level does not consume
// the attribute: the module stays visible in the record.
func TestModuleAttrIsStillRendered(t *testing.T) {
	t.Parallel()
	levels, _ := ParseLevels("mcp=debug,info")
	log, buf, _ := newTestLogger(t, &Options{Levels: levels})
	log.With(DefaultModuleKey, "mcp").Debug("hello")
	if !strings.Contains(buf.String(), "module=mcp") {
		t.Fatalf("record = %q, want the module attribute", buf.String())
	}
}

// TestModuleInsideGroupDoesNotSelectLevel proves the module key only counts
// at the top level; a "module" key nested in a group names something else.
func TestModuleInsideGroupDoesNotSelectLevel(t *testing.T) {
	t.Parallel()
	levels, _ := ParseLevels("mcp=debug,info")
	log, buf, _ := newTestLogger(t, &Options{Levels: levels})
	log.WithGroup("req").With(DefaultModuleKey, "mcp").Debug("must not be emitted")
	if buf.Len() != 0 {
		t.Fatalf("output = %q, want the record suppressed at the default level", buf.String())
	}
}

// TestCustomModuleKey proves the key is configurable.
func TestCustomModuleKey(t *testing.T) {
	t.Parallel()
	levels, _ := ParseLevels("mcp=debug,info")
	log, buf, _ := newTestLogger(t, &Options{Levels: levels, ModuleKey: "component"})
	log.With("component", "mcp").Debug("emitted")
	if !strings.Contains(buf.String(), "emitted") {
		t.Fatalf("output = %q, want the record at the module level", buf.String())
	}
}

// TestColorWrapsOnlyTheLevel proves color is applied to the level name and
// that NoColor removes it entirely.
func TestColorWrapsOnlyTheLevel(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	slog.New(New(buf, &Options{})).Warn("hello", "k", "v")
	colored := buf.String()
	if !strings.Contains(colored, levelColors[slog.LevelWarn]+"WARN"+ansiReset) {
		t.Fatalf("record = %q, want the level wrapped in color", colored)
	}
	if strings.Contains(strings.TrimPrefix(colored, "time="), "\033[36m") {
		t.Fatalf("record = %q, want only the level colored", colored)
	}

	plain := &bytes.Buffer{}
	slog.New(New(plain, &Options{NoColor: true})).Warn("hello", "k", "v")
	if strings.Contains(plain.String(), "\033[") {
		t.Fatalf("NoColor record = %q, want no escapes", plain.String())
	}
}

// TestRecordHasTimestamp proves every record carries the time field, which
// is what makes the console output usable.
func TestRecordHasTimestamp(t *testing.T) {
	t.Parallel()
	log, buf, _ := newTestLogger(t, nil)
	log.Info("hello")
	if !strings.HasPrefix(buf.String(), "time=") {
		t.Fatalf("record = %q, want a leading time field", buf.String())
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Fatalf("record = %q, want a trailing newline", buf.String())
	}
}

// TestValuesAreQuotedWhenAmbiguous proves a value carrying spaces or an
// equals sign cannot break key=value scanning.
func TestValuesAreQuotedWhenAmbiguous(t *testing.T) {
	t.Parallel()
	log, buf, _ := newTestLogger(t, nil)
	log.Info("hello", "path", "/tmp/a b", "empty", "", "eq", "a=b")
	out := buf.String()
	for _, want := range []string{`path="/tmp/a b"`, `empty=""`, `eq="a=b"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("record = %q, want %q", out, want)
		}
	}
}

// TestFilterSuppressesNonMatchingRecords proves the content filter keeps
// matching records at or below its level and always keeps severer ones.
func TestFilterSuppressesNonMatchingRecords(t *testing.T) {
	t.Parallel()
	levels := NewLevels(slog.LevelDebug)
	log, buf, h := newTestLogger(t, &Options{Levels: levels})
	h.SetFilter("checkpoint", slog.LevelInfo)

	log.Info("checkpoint scheduled")
	log.Info("unrelated message")
	log.Info("matched by attr", "stage", "checkpoint.cas")
	log.Warn("severe records bypass the filter")

	out := buf.String()
	for _, want := range []string{"checkpoint scheduled", "matched by attr", "severe records bypass"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output = %q, want %q", out, want)
		}
	}
	if strings.Contains(out, "unrelated message") {
		t.Fatalf("output = %q, must not contain the unmatched record", out)
	}
}

// TestFilterReachesDerivedHandlers proves SetFilter on the root applies to
// loggers derived after it, which is how a component logger inherits it.
func TestFilterReachesDerivedHandlers(t *testing.T) {
	t.Parallel()
	log, buf, h := newTestLogger(t, &Options{Levels: NewLevels(slog.LevelDebug)})
	component := log.With(DefaultModuleKey, "mcp")
	h.SetFilter("keep", slog.LevelInfo)

	component.Info("keep this")
	component.Info("drop this")

	out := buf.String()
	if !strings.Contains(out, "keep this") {
		t.Fatalf("output = %q, want the matching record", out)
	}
	if strings.Contains(out, "drop this") {
		t.Fatalf("output = %q, must not contain the unmatched record", out)
	}
}

// TestConcurrentRecordsAreNotInterleaved proves derived handlers share the
// writer lock, so a record is one unsplit line even under concurrency.
func TestConcurrentRecordsAreNotInterleaved(t *testing.T) {
	t.Parallel()
	log, buf, _ := newTestLogger(t, nil)
	var wg sync.WaitGroup
	const writers, each = 8, 25
	for i := range writers {
		wg.Go(func() {
			component := log.With(DefaultModuleKey, "w", "writer", i)
			for range each {
				component.Info("concurrent record", "payload", "aaaaaaaaaaaaaaaaaaaaaaaa")
			}
		})
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != writers*each {
		t.Fatalf("lines = %d, want %d", len(lines), writers*each)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "time=") || !strings.Contains(line, `msg="concurrent record"`) {
			t.Fatalf("interleaved line: %q", line)
		}
	}
}

// TestEnabledReflectsModuleLevel proves the level is resolved when the
// module is bound, which is what lets slog skip building a record at all.
func TestEnabledReflectsModuleLevel(t *testing.T) {
	t.Parallel()
	levels, _ := ParseLevels("cli=error,debug")
	log, _, h := newTestLogger(t, &Options{Levels: levels})
	if h.Level() != slog.LevelDebug {
		t.Fatalf("root level = %v, want debug", h.Level())
	}
	cli := log.With(DefaultModuleKey, "cli")
	if cli.Enabled(t.Context(), slog.LevelWarn) {
		t.Fatal("cli logger is enabled at warn, want error only")
	}
	if !cli.Enabled(t.Context(), slog.LevelError) {
		t.Fatal("cli logger is not enabled at error")
	}
	if !log.Enabled(t.Context(), slog.LevelDebug) {
		t.Fatal("root logger is not enabled at debug")
	}
}
