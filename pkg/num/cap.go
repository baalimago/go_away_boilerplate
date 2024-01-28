package num

import "golang.org/x/exp/constraints"

type Number interface {
	constraints.Integer | constraints.Float | constraints.Signed | constraints.Unsigned
}

// Cap n so that it's more than or equal to min, and less than or equal to max
func Cap[N Number](n, min, max N) (ret N) {
	if n <= min {
		return min
	}

	if n >= max {
		return max
	}

	return n
}
