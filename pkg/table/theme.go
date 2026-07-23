package table

import (
	"os"

	"github.com/baalimago/go_away_boilerplate/pkg/misc"
)

// Theme holds the color palette and default page size for a table.
// Zero-value fields mean "use default."
type Theme struct {
	Primary   string
	Secondary string
	Breadtext string
	Items     int
}

// DefaultTheme returns the built-in muted gray-blue palette.
func DefaultTheme() Theme {
	return Theme{
		Primary:   "\u001b[38;2;110;130;150m",
		Secondary: "\u001b[38;2;140;165;190m",
		Breadtext: "\u001b[38;2;200;210;220m",
		Items:     10,
	}
}

const ansiReset = "\u001b[0m"

// NoColor reports whether color output should be disabled, respecting the
// NO_COLOR environment variable.
func NoColor() bool {
	return misc.Truthy(os.Getenv("NO_COLOR"))
}

// Colorize wraps s with the given ANSI color code unless NO_COLOR is set or
// color is empty.
func Colorize(color, s string) string {
	if NoColor() || color == "" {
		return s
	}
	return color + s + ansiReset
}
