package testboil

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// CreateTestFile or fatal trying. Since it t.Fatalf on failure, return value
// won't matter. So the return value can be assumed to never be nil
func CreateTestFile(t *testing.T, fileName string) *os.File {
	file, err := os.Create(fmt.Sprintf("%v/%v", t.TempDir(), fileName))
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	return file
}

func FailTestIfDiff[C comparable](t *testing.T, got, want C) {
	t.Helper()
	if got != want {
		t.Fatalf("Test failed, diff:\nwant: %-10v\n got: %-10v\n", want, got)
	}
}

func AssertStringContains(t *testing.T, want, got string) {
	if !strings.Contains(got, want) {
		t.Fatalf("expected: '%v' to contain substring '%v'", got, want)
	}
}
