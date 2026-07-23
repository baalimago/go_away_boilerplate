package table

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"
	"unsafe"
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

// TermWidth returns the current terminal width in columns.
//
// Prefers the COLUMNS environment variable if present and positive. Falls back
// to ioctl(TIOCGWINSZ). When ioctl fails (e.g. in CI without a TTY), returns 80
// as a sane default.
func TermWidth() (int, error) {
	if c := os.Getenv("COLUMNS"); c != "" {
		if n, err := strconv.Atoi(c); err == nil && n > 0 {
			return n, nil
		}
	}

	ws := &struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}{}

	retCode, _, _ := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(syscall.Stderr),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(ws)),
	)

	if int(retCode) == -1 {
		return 80, nil
	}
	if ws.Col == 0 {
		return 80, nil
	}
	return int(ws.Col), nil
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
func WidthAppropriateStringTruncColored(toShorten, prefix, prefixColor, truncColor string, padding int) (string, error) {
	toShorten = strings.ReplaceAll(toShorten, "\n", "\\n")
	toShorten = strings.ReplaceAll(toShorten, "\t", "\\t")

	termWidth, err := TermWidth()
	if err != nil {
		return "", fmt.Errorf("get term width: %w", err)
	}

	return fillRemainderOfTermWidthColored(prefix, toShorten, prefixColor, truncColor, termWidth, padding), nil
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
