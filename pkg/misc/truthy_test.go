package misc_test

import (
	"testing"

	"github.com/baalimago/go_away_boilerplate/pkg/misc"
)

func TestTruthy(t *testing.T) {
	tests := []struct {
		name string
		arg  any
		want bool
	}{
		{"bool true", true, true},
		{"bool false", false, false},
		{"int 1", 1, true},
		{"int 0", 0, false},
		{"string 'true'", "true", true},
		{"string 'TRUE' with spaces", " TRUE ", true},
		{"string '1'", "1", true},
		{"string empty", "", false},
		{"string random", "random", false},
		{"nil", nil, false},
		{"some struct", struct{ somefield string }{somefield: "test"}, true},
		// Add more cases as necessary
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := misc.Truthy(tt.arg); got != tt.want {
				t.Errorf("Truthy(%v) = %v, want %v", tt.arg, got, tt.want)
			}
		})
	}
}
