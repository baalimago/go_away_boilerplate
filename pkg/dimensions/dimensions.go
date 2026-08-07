// Package dimensions reads Unix terminal sizes and observes resize events.
//
// It is the single terminal-dimension implementation for this module: the
// ioctl query and the fallback policy live only here. No other package in
// this module queries TIOCGWINSZ, and no package shells out to tmux or any
// external command to learn the terminal size.
//
// A Dimensions value carries width and height together, so callers can use
// one resolved size for a complete render operation.
package dimensions

import "errors"

// Dimensions is the size of a terminal in character cells.
type Dimensions struct {
	// Width is the number of columns.
	Width int
	// Height is the number of rows.
	Height int
}

// Fallback is the documented safe size used when a terminal query fails or
// returns an unusable size and no valid snapshot exists. It preserves the
// historical table.TermWidth fallback of 80 columns and adds a height of 24.
var Fallback = Dimensions{Width: 80, Height: 24}

// ErrUnavailable is wrapped by every terminal-size read failure: ioctl
// errors, zero width or height, and unsupported platforms. Use errors.Is to
// detect "this file descriptor has no usable terminal size".
var ErrUnavailable = errors.New("terminal dimensions unavailable")

// Reader returns the current size of one terminal. Implementations must
// return width and height together from one query and must never fabricate a
// size: on failure they return zero Dimensions and a wrapped ErrUnavailable.
// The Viewer calls a Reader serially, never concurrently.
type Reader func() (Dimensions, error)
