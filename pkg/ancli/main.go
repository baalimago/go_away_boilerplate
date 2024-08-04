// ancli is a simple suite of reusable ansi cli functions
// It's like ansi cli, and the "c" sounds like "si", so it becomes
// an"si"li? get it? It's very funny.
package ancli

import (
	"fmt"
	"io"
	"os"
)

type colorCode int

const (
	RED colorCode = iota + 31
	GREEN
	YELLOW
	BLUE
	MAGENTA
	CYAN
)

var useColor = os.Getenv("NO_COLOR") != "true"
var Newline = false

func ColoredMessage(cc colorCode, msg string) string {
	return fmt.Sprintf("\x1b[%dm%v\x1b[0m", cc, msg)
}

func printStatus(out io.Writer, status, msg string, color colorCode) {
	if useColor {
		status = ColoredMessage(color, status)
	}
	newline := ""
	if Newline {
		newline = "\n"
	}
	fmt.Fprintf(out, "%v: %v%v", status, msg, newline)
}

func PrintErr(msg string) {
	printStatus(os.Stderr, "error", msg, RED)
}

func PrintfErr(msg string, a ...string) {
	PrintErr(fmt.Sprintf(msg, a))
}

func PrintOK(msg string) {
	printStatus(os.Stdout, "ok", msg, GREEN)
}

func PrintfOK(msg string, a ...string) {
	PrintOK(fmt.Sprintf(msg, a))
}

func PrintWarn(msg string) {
	printStatus(os.Stdout, "warning", msg, YELLOW)
}

func PrintfWarn(msg string, a ...string) {
	PrintWarn(fmt.Sprintf(msg, a))
}

func PrintNotice(msg string) {
	printStatus(os.Stdout, "notice", msg, CYAN)
}

func PrintfNotice(msg string, a ...string) {
	PrintNotice(fmt.Sprintf(msg, a))
}
