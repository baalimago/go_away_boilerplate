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
)

var useColor = os.Getenv("NO_COLOR") != "true"

func coloredMessage(cc colorCode, msg string) string {
	return fmt.Sprintf("\x1b[%dm%v\x1b[0m", cc, msg)
}

func printStatus(out io.Writer, status, msg string, color colorCode) {
	if useColor {
		status = coloredMessage(color, status)
	}
	fmt.Fprintf(out, "%v: %v", status, msg)
}

func printErr(msg string) {
	printStatus(os.Stderr, "error", msg, RED)
}

func printOK(msg string) {
	printStatus(os.Stdout, "ok", msg, GREEN)
}

func printWarn(msg string) {
	printStatus(os.Stdout, "warning", msg, YELLOW)
}
