package table

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/baalimago/go_away_boilerplate/pkg/dimensions"
)

var ansiEscapeSeq = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// ClearLine writes a carriage return followed by the ANSI "clear to end of
// line" escape, so the next write starts at column 0 on a clean line.
func ClearLine(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprint(w, "\r\x1b[K")
}

// ClearTermTo clears upTo lines upwards, leaving the cursor at column 0 of the
// last cleared line. Each line is cleared via ClearLine.
func ClearTermTo(w io.Writer, upTo int) error {
	if w == nil {
		w = os.Stdout
	}
	for upTo > 0 {
		ClearLine(w)
		fmt.Fprintf(w, "\033[%dA", 1)
		upTo--
	}
	ClearLine(w)
	return nil
}

// TermWidth returns the current terminal width in columns, measured on
// stderr through the shared dimensions package.
//
// TermWidth is a compatibility wrapper: it preserves the historical silent
// fallback. When the terminal size is unavailable (stderr is not a terminal,
// the ioctl fails, or the terminal reports an unusable size), it returns
// dimensions.Fallback.Width (80) and a nil error, exactly as the legacy
// implementation did. The ioctl query and the fallback policy live in
// pkg/dimensions; this wrapper only maps the wrapped ErrUnavailable to the
// legacy fallback value. Callers that need terminal awareness use
// pkg/dimensions directly and check errors.Is(err, dimensions.ErrUnavailable).
func TermWidth() (int, error) {
	return termWidth(dimensions.DefaultReader(os.Stderr.Fd()))
}

// termWidth is the injectable core of TermWidth. It projects a dimensions
// read onto the width and maps every failure to the documented fallback, so
// compatibility callers always receive usable output.
func termWidth(read dimensions.Reader) (int, error) {
	d, err := read()
	if err != nil {
		return dimensions.Fallback.Width, nil
	}
	return d.Width, nil
}

// WidthAppropriateStringTrunc is a convenience wrapper around
// WidthAppropriateStringTruncColored with empty color arguments.
func WidthAppropriateStringTrunc(toShorten, prefix string, padding int) (string, error) {
	return WidthAppropriateStringTruncColored(toShorten, prefix, "", "", padding)
}

// visibleRuneCount returns the number of visible runes in s after stripping
// ANSI SGR escape sequences.
func visibleRuneCount(s string) int {
	clean := ansiEscapeSeq.ReplaceAllString(s, "")
	return utf8.RuneCountInString(clean)
}

// WidthAppropriateStringTruncColored truncates toShorten to fit within the
// terminal width, prepending prefix and inserting a " ... " infix when
// truncation occurs. prefixColor and truncColor are raw ANSI sequences (or
// empty). Colors are disabled when NO_COLOR is truthy.
//
// The width comes from TermWidth, so the legacy silent fallback applies when
// no terminal size is available. Callers that already hold a dimensions
// snapshot use WidthAppropriateStringTruncColoredWithWidth to avoid a second
// terminal query.
func WidthAppropriateStringTruncColored(toShorten, prefix, prefixColor, truncColor string, padding int) (string, error) {
	termWidth, err := TermWidth()
	if err != nil {
		return "", fmt.Errorf("get term width: %w", err)
	}
	return WidthAppropriateStringTruncColoredWithWidth(toShorten, prefix, prefixColor, truncColor, padding, termWidth), nil
}

// WidthAppropriateStringTruncWithWidth is a convenience wrapper around
// WidthAppropriateStringTruncColoredWithWidth with empty color arguments.
// width is used exactly as given; no terminal query is performed.
func WidthAppropriateStringTruncWithWidth(toShorten, prefix string, padding, width int) string {
	return WidthAppropriateStringTruncColoredWithWidth(toShorten, prefix, "", "", padding, width)
}

// WidthAppropriateStringTruncColoredWithWidth truncates toShorten to fit
// within width columns, prepending prefix and inserting a " ... " infix when
// truncation occurs. It uses width exactly as supplied and never performs a
// terminal query, so a caller can render one complete operation with a single
// resolved dimension set. A zero or negative width clamps to the prefix only.
func WidthAppropriateStringTruncColoredWithWidth(toShorten, prefix, prefixColor, truncColor string, padding, width int) string {
	toShorten = strings.ReplaceAll(toShorten, "\n", "\\n")
	toShorten = strings.ReplaceAll(toShorten, "\t", "\\t")

	return fillRemainderOfTermWidthColored(prefix, toShorten, prefixColor, truncColor, width, padding)
}

func fillRemainderOfTermWidthColored(prefix, remainder, prefixColor, truncColor string, termWidth, padding int) string {
	infix := " ... "
	infixLen := visibleRuneCount(infix)

	remainingWidth := max(termWidth-visibleRuneCount(prefix)-padding, 0)
	widthAdjustedRemainder := ""
	r := []rune(remainder)
	if remainingWidth == 0 {
		widthAdjustedRemainder = ""
	} else if len(r) <= remainingWidth {
		widthAdjustedRemainder = remainder
	} else if remainingWidth <= infixLen {
		widthAdjustedRemainder = string(r[:remainingWidth])
	} else {
		avail := remainingWidth - infixLen
		startLen := avail / 2
		endLen := max(avail-startLen, 0)
		if startLen < 0 {
			startLen = 0
		}
		if startLen > len(r) {
			startLen = len(r)
		}
		if endLen > len(r)-startLen {
			endLen = len(r) - startLen
		}
		endStart := max(len(r)-endLen, 0)

		widthAdjustedRemainder = string(r[:startLen]) +
			infix +
			string(r[endStart:])
	}

	return Colorize(prefixColor, prefix) + Colorize(truncColor, widthAdjustedRemainder)
}
