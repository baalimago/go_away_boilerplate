package misc_test

import (
	"testing"

	"github.com/baalimago/go_away_boilerplate/pkg/misc"
)

func TestPointer(t *testing.T) {
	tests := []struct {
		name string
		arg  interface{}
		want interface{}
	}{
		{"int", 42, 42},
		{"string", "hello", "hello"},
		{"bool true", true, true},
		{"bool false", false, false},
		{"float64", 3.14, 3.14},
		{"nil interface", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := misc.Pointer(tt.arg)
			if got == nil {
				t.Errorf("Pointer(%v) returned nil",
					tt.arg)
			}
			if *got != tt.want {
				t.Errorf("Pointer(%v) = %v, "+
					"want %v", tt.arg, *got,
					tt.want)
			}
		})
	}
}
