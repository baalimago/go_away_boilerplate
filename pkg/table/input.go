package table

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
)

const defaultTTYPath = "/dev/tty"

// ReadUserInput reads a single line from the TTY and returns the trimmed
// string. It returns ErrUserInitiatedExit on SIGINT or when the user types "q"
// or "quit".
func ReadUserInput() (string, error) {
	return readUserInput()
}

// ReadUserInputFrom reads a single line from r and returns the trimmed string.
// When r is nil, delegates to ReadUserInput (TTY-based, with SIGINT handling).
// Returns ErrUserInitiatedExit when the input is "q" or "quit". Read errors
// (including io.EOF) are wrapped and returned.
func ReadUserInputFrom(r io.Reader) (string, error) {
	if r == nil {
		return ReadUserInput()
	}
	// Read one byte at a time to avoid consuming ahead. A bufio.Reader
	// wrapping the same underlying reader may have already buffered data;
	// byte-by-byte reads ensure we don't advance past the intended line.
	var buf strings.Builder
	b := make([]byte, 1)
	for {
		_, err := r.Read(b)
		if err != nil {
			if buf.Len() > 0 {
				break
			}
			return "", fmt.Errorf("read macro input: %w", err)
		}
		if b[0] == '\n' {
			break
		}
		buf.WriteByte(b[0])
	}
	trimmedInput := strings.TrimSpace(buf.String())
	quitters := []string{"q", "quit"}
	if slices.Contains(quitters, trimmedInput) {
		return "", ErrUserInitiatedExit
	}
	return trimmedInput, nil
}

func readUserInput() (string, error) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	defer signal.Stop(sigChan)
	inputChan := make(chan string)
	errChan := make(chan error)

	go func() {
		ttyPath := os.Getenv("TTY")
		if ttyPath == "" {
			ttyPath = defaultTTYPath
		}

		tty, err := os.Open(ttyPath)
		if err != nil {
			errChan <- fmt.Errorf("cannot open terminal %q: %w", ttyPath, err)
			return
		}
		defer tty.Close()

		reader := bufio.NewReader(tty)
		userInput, err := reader.ReadString('\n')
		if err != nil {
			errChan <- fmt.Errorf("read from terminal %q: %w", ttyPath, err)
			return
		}
		inputChan <- userInput
	}()

	select {
	case <-sigChan:
		return "", ErrUserInitiatedExit
	case err := <-errChan:
		return "", fmt.Errorf("failed to read user input: %w", err)
	case userInput, open := <-inputChan:
		if open {
			trimmedInput := strings.TrimSpace(userInput)
			quitters := []string{"q", "quit"}
			if slices.Contains(quitters, trimmedInput) {
				return "", ErrUserInitiatedExit
			}
			return trimmedInput, nil
		}
		return "", fmt.Errorf("user input channel closed: %w", errors.New("channel closed"))
	}
}
