// Package slogcolor provides a slog.Handler that writes colored text output
// with per-module levels.
//
// Output matches [slog.TextHandler] except that the level name is wrapped in
// ANSI color escapes:
//
//	DEBUG → Cyan
//	INFO  → Green
//	WARN  → Yellow
//	ERROR → Red
//
// Each record may name a module through an attribute (see DefaultModuleKey),
// and each module can carry its own level:
//
//	levels, _ := slogcolor.ParseLevels("cli=warn,mcp=debug,info")
//	h := slogcolor.New(os.Stderr, &slogcolor.Options{Levels: levels})
//	log := slog.New(h).With(slogcolor.DefaultModuleKey, "mcp") // debug level
//
// The module level applies from the moment the attribute is bound, so
// binding it once per component is enough; slog consults Enabled before it
// builds a record, which is why the module has to be a bound attribute
// rather than a call-site one.
package slogcolor

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
)

// DefaultTimeFormat is the layout of the time= field.
const DefaultTimeFormat = "2006-01-02T15:04:05.000-0700"

var levelColors = map[slog.Level]string{
	slog.LevelDebug: "\033[36m", // Cyan
	slog.LevelInfo:  "\033[32m", // Green
	slog.LevelWarn:  "\033[33m", // Yellow
	slog.LevelError: "\033[31m", // Red
}

const ansiReset = "\033[0m"

// Options configure a Handler. A nil *Options means the Info level for every
// module, color enabled, and DefaultTimeFormat.
type Options struct {
	// Levels holds the default level and the per-module overrides. Nil means
	// Info everywhere.
	Levels *Levels
	// ModuleKey is the attribute key naming a record's module. Empty means
	// DefaultModuleKey.
	ModuleKey string
	// NoColor writes plain level names. Set it from the NO_COLOR convention:
	// any non-empty NO_COLOR value disables color.
	NoColor bool
	// TimeFormat overrides DefaultTimeFormat.
	TimeFormat string
	// AddSource is accepted for symmetry with slog.HandlerOptions. It is not
	// used yet.
	AddSource bool
}

// Handler writes colored, module-leveled text records.
//
// A Handler is safe for concurrent use. Derived handlers from WithAttrs and
// WithGroup share the writer and its lock, so records never interleave.
type Handler struct {
	mu   *sync.Mutex
	w    io.Writer
	opts Options

	// level is this handler's effective minimum, resolved from the module
	// bound so far.
	level slog.Level

	attrs  []slog.Attr // group-qualified at bind time
	groups []string

	filter      *filterState
	moduleKeyed string
}

// filterState is shared by every derived handler so SetFilter on the root
// reaches records logged through a component's own logger.
type filterState struct {
	mu    sync.RWMutex
	text  string
	level slog.Level
}

// New returns a Handler writing to w.
func New(w io.Writer, opts *Options) *Handler {
	if opts == nil {
		opts = &Options{}
	}
	o := *opts
	if o.ModuleKey == "" {
		o.ModuleKey = DefaultModuleKey
	}
	if o.TimeFormat == "" {
		o.TimeFormat = DefaultTimeFormat
	}
	if o.Levels == nil {
		o.Levels = NewLevels(slog.LevelInfo)
	}
	return &Handler{
		mu:     &sync.Mutex{},
		w:      w,
		opts:   o,
		level:  o.Levels.Default(),
		filter: &filterState{},
	}
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	if !h.passesFilter(r) {
		return nil
	}

	buf := make([]byte, 0, 256)

	if !r.Time.IsZero() {
		buf = append(buf, "time="...)
		buf = append(buf, r.Time.Format(h.opts.TimeFormat)...)
		buf = append(buf, ' ')
	}

	buf = append(buf, "level="...)
	if h.opts.NoColor {
		buf = append(buf, r.Level.String()...)
	} else {
		color := levelColors[r.Level]
		if color == "" {
			color = ansiReset
		}
		buf = append(buf, color...)
		buf = append(buf, r.Level.String()...)
		buf = append(buf, ansiReset...)
	}
	buf = append(buf, ' ')

	buf = append(buf, "msg="...)
	buf = strconv.AppendQuote(buf, r.Message)

	// Attributes bound through WithAttrs are already group-qualified;
	// call-site attributes take the currently open group prefix.
	for _, a := range h.attrs {
		buf = appendAttr(buf, "", a)
	}
	prefix := groupPrefix(h.groups)
	r.Attrs(func(a slog.Attr) bool {
		buf = appendAttr(buf, prefix, a)
		return true
	})

	buf = append(buf, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(buf)
	return err
}

// WithAttrs binds attrs to every later record. Binding the module attribute
// switches this handler to that module's level.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	out := *h
	prefix := groupPrefix(h.groups)
	bound := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	bound = append(bound, h.attrs...)
	for _, a := range attrs {
		bound = append(bound, slog.Attr{Key: prefix + a.Key, Value: a.Value})
		// The module attribute selects the level, but only at the top
		// level: a "module" key nested inside a group names something else.
		if prefix == "" && a.Key == h.opts.ModuleKey {
			module := a.Value.Resolve().String()
			out.level = h.opts.Levels.For(module)
			out.moduleKeyed = module
		}
	}
	out.attrs = bound
	return &out
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	out := *h
	out.groups = append(append([]string(nil), h.groups...), name)
	return &out
}

// Module reports the module bound to this handler, if any.
func (h *Handler) Module() string { return h.moduleKeyed }

// Level reports this handler's effective minimum level.
func (h *Handler) Level() slog.Level { return h.level }

// SetFilter configures message-content filtering.
//
// When filter is non-empty, records at or below level are written only if
// their message or an attribute value contains filter as a substring.
// Records above level are always written. An empty filter disables it.
//
// The setting is shared with every handler derived through WithAttrs and
// WithGroup, so setting it on the root reaches component loggers too.
func (h *Handler) SetFilter(filter string, level slog.Level) {
	h.filter.mu.Lock()
	defer h.filter.mu.Unlock()
	h.filter.text = filter
	h.filter.level = level
}

// passesFilter reports whether r survives the content filter.
func (h *Handler) passesFilter(r slog.Record) bool {
	h.filter.mu.RLock()
	text, level := h.filter.text, h.filter.level
	h.filter.mu.RUnlock()
	if text == "" || r.Level > level {
		return true
	}
	if strings.Contains(r.Message, text) {
		return true
	}
	for _, a := range h.attrs {
		if strings.Contains(a.Value.Resolve().String(), text) {
			return true
		}
	}
	found := false
	r.Attrs(func(a slog.Attr) bool {
		if strings.Contains(a.Value.Resolve().String(), text) {
			found = true
			return false
		}
		return true
	})
	return found
}

// groupPrefix joins open groups into the dotted key prefix slog specifies.
func groupPrefix(groups []string) string {
	if len(groups) == 0 {
		return ""
	}
	return strings.Join(groups, ".") + "."
}

// appendAttr writes one attribute, flattening groups and resolving
// LogValuer values so a value hidden behind LogValue is rendered rather than
// printed as its opaque wrapper.
func appendAttr(buf []byte, prefix string, a slog.Attr) []byte {
	value := a.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		group := value.Group()
		if a.Key != "" {
			prefix += a.Key + "."
		}
		for _, member := range group {
			buf = appendAttr(buf, prefix, member)
		}
		return buf
	}
	if a.Equal(slog.Attr{}) {
		return buf
	}
	buf = append(buf, ' ')
	buf = append(buf, prefix...)
	buf = append(buf, a.Key...)
	buf = append(buf, '=')
	return appendValue(buf, value.String())
}

// appendValue quotes a value that would otherwise break key=value scanning.
func appendValue(buf []byte, s string) []byte {
	if s == "" {
		return append(buf, `""`...)
	}
	if strings.ContainsAny(s, " \t\n\"=") {
		return strconv.AppendQuote(buf, s)
	}
	return append(buf, s...)
}
