package testboil_test

import (
	"testing"

	"github.com/baalimago/go_away_boilerplate/pkg/testboil"
)

func Test_CreateTestFile(t *testing.T) {
	t.Run("it should create a test file", func(t *testing.T) {
		got := testboil.CreateTestFile(t, "somename")
		if got == nil {
			t.Fatal("expected to find a file, got nil")
		}
	})
}
