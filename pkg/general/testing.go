package general

import (
	"fmt"
	"os"
	"testing"
)

// CreateTestFile or fatal trying. Returns nil on error and Fatalf's the test
func CreateTestFile(t *testing.T, fileName string) *os.File {
	file, err := os.Open(fmt.Sprintf("%v/%v", t, fileName))
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
		return nil
	}
	return file
}
