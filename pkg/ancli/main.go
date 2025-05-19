// ancli is a simple suite of reusable ansi cli functions
// It's like ansi cli, and the "c" sounds like "si", so it becomes
// an"si"li? get it? It's very funny.
package ancli

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/baalimago/go_away_boilerplate/pkg/misc"
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

var (
	useColor      = os.Getenv("NO_COLOR") != "true"
	printWarnings = !misc.Truthy(os.Getenv("NO_WARNINGS"))
	Newline       = false || strings.ToLower(os.Getenv("ANCLI_NEWLINE")) == "true"
	SlogIt        = false
	slogger       *slog.Logger
	slogMu        = sync.Mutex{}
)

func SetupSlog() {
	slogMu.Lock()
	defer slogMu.Unlock()
	slogger = slog.New(&ansiprint{})
	SlogIt = true
}

func ColoredMessage(cc colorCode, msg string) string {
	return fmt.Sprintf("\x1b[%dm%v\x1b[0m", cc, msg)
}

func printStatus(out io.Writer, status, msg string, color colorCode) {
	slogMu.Lock()
	defer slogMu.Unlock()
	rawStatus := status
	if useColor {
		status = ColoredMessage(color, status)
	}
	newline := ""
	if Newline {
		newline = "\n"
	}
	if SlogIt {
		if slogger == nil {
			SlogIt = false
			PrintErr("you have to run ancli.SetupSlog in order to use slog printing, defaulting to normal print")
		}
		if slogger != nil {
			// Always newline slog messages
			fmsg := fmt.Sprintf("%v: %v\n", status, msg)
			switch rawStatus {
			case "ok", "notice":
				slogger.Info(fmsg)
			case "error":
				slogger.Error(fmsg)
			case "warning":
				slogger.Warn(fmsg)
			default:
				slogger.Warn(fmt.Sprintf("failed to find status for: '%v', msg is: %v", status, fmsg))
			}
		}
	} else {
		fmt.Fprintf(out, "%v: %v%v", status, msg, newline)
	}
}

func PrintErr(msg string) {
	printStatus(os.Stderr, "error", msg, RED)
}

func PrintfErr(msg string, a ...any) {
	PrintErr(fmt.Sprintf(msg, a...))
}

func Errf(msg string, a ...any) {
	PrintErr(fmt.Sprintf(msg, a...))
}

func PrintOK(msg string) {
	printStatus(os.Stdout, "ok", msg, GREEN)
}

func PrintfOK(msg string, a ...any) {
	PrintOK(fmt.Sprintf(msg, a...))
}

func Okf(msg string, a ...any) {
	PrintOK(fmt.Sprintf(msg, a...))
}

func PrintWarn(msg string) {
	if !printWarnings {
		return
	}
	printStatus(os.Stdout, "warning", msg, YELLOW)
}

func PrintfWarn(msg string, a ...any) {
	PrintWarn(fmt.Sprintf(msg, a...))
}

func Warnf(msg string, a ...any) {
	PrintWarn(fmt.Sprintf(msg, a...))
}

func PrintNotice(msg string) {
	printStatus(os.Stdout, "notice", msg, CYAN)
}

func PrintfNotice(msg string, a ...any) {
	PrintNotice(fmt.Sprintf(msg, a...))
}

func Noticef(msg string, a ...any) {
	PrintNotice(fmt.Sprintf(msg, a...))
}
