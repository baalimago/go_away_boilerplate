package num

import "golang.org/x/exp/constraints"

type Number interface {
	constraints.Integer | constraints.Float
}

func Cap[N Number](n, min, max N) (ret N) {
	if n < min {
		return min
	}

	if n > max {
		return max
	}

	return n
}
