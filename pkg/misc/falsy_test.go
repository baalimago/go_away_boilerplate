package misc_test

import (
	"testing"

	"github.com/baalimago/go_away_boilerplate/pkg/misc"
)

func TestFalsy(t *testing.T) {
	tests := []struct {
		name string
		arg  any
		want bool
	}{
		{"bool true", true, false},
		{"bool false", false, true},
		{"int 1", 1, false},
		{"int 0", 0, true},
		{"string 'true'", "true", false},
		{"string 'TRUE' with spaces", " TRUE ", false},
		{"string '1'", "1", false},
		{"string empty", "", true},
		{"string random", "random", false},
		{"nil", nil, true},
		{"some struct", struct{ somefield string }{somefield: "test"}, false},
		// Add more cases as necessary
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := misc.Falsy(tt.arg); got != tt.want {
				t.Errorf("Falsy(%v) = %v, want %v", tt.arg, got, tt.want)
			}
		})
	}
}
