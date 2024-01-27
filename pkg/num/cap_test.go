package num

import (
	"testing"
)

type testCase struct {
	name                  string
	n, min, max, expected int
}

func failOnDiff[N Number](t *testing.T, a, b N) {
	if a != b {
		t.Errorf("Expected %v, got %v", a, b)
	}
}

func TestCap(t *testing.T) {
	tests := []testCase{
		{"Cap below min", 5, 10, 20, 10},
		{"Cap above max", 25, 10, 20, 20},
		{"Within range", 15, 10, 20, 15},
		{"Equal to min", 10, 10, 20, 10},
		{"Equal to max", 20, 10, 20, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failOnDiff(t, int8(tt.expected), Cap(int8(tt.n), int8(tt.min), int8(tt.max)))
			failOnDiff(t, int16(tt.expected), Cap(int16(tt.n), int16(tt.min), int16(tt.max)))
			failOnDiff(t, int32(tt.expected), Cap(int32(tt.n), int32(tt.min), int32(tt.max)))
			failOnDiff(t, int64(tt.expected), Cap(int64(tt.n), int64(tt.min), int64(tt.max)))
			failOnDiff(t, uint8(tt.expected), Cap(uint8(tt.n), uint8(tt.min), uint8(tt.max)))
			failOnDiff(t, uint16(tt.expected), Cap(uint16(tt.n), uint16(tt.min), uint16(tt.max)))
			failOnDiff(t, uint32(tt.expected), Cap(uint32(tt.n), uint32(tt.min), uint32(tt.max)))
			failOnDiff(t, uint64(tt.expected), Cap(uint64(tt.n), uint64(tt.min), uint64(tt.max)))
			failOnDiff(t, float32(tt.expected), Cap(float32(tt.n), float32(tt.min), float32(tt.max)))
			failOnDiff(t, float64(tt.expected), Cap(float64(tt.n), float64(tt.min), float64(tt.max)))
		})
	}
}
