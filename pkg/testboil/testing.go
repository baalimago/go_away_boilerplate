package testboil

import (
	"fmt"
	"os"
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
