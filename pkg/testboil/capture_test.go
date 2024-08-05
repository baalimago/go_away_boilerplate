package testboil

import (
	"fmt"
	"os"
	"testing"
)

func Test_CaptureStdout(t *testing.T) {
	t.Run("it should capture stdout", func(t *testing.T) {
		want := "hello"
		stdoutPrinter := func(t *testing.T) {
			t.Helper()
			fmt.Print(want)
		}
		got := CaptureStdout(t, func(t *testing.T) {
			t.Helper()
			stdoutPrinter(t)
		})
		if got != want {
			t.Fatalf("expected: %v, got: %v", want, got)
		}
	})
}

func Test_CaptureStderr(t *testing.T) {
	t.Run("it should capture stdout", func(t *testing.T) {
		want := "hello"
		stderrPrinter := func(t *testing.T) {
			t.Helper()
			fmt.Fprint(os.Stderr, want)
		}
		got := CaptureStdrer(t, func(t *testing.T) {
			t.Helper()
			stderrPrinter(t)
		})
		if got != want {
			t.Fatalf("expected: %v, got: %v", want, got)
		}
	})
}
