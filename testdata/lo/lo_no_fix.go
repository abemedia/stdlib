package slices

import (
	"github.com/samber/lo"
)

func less(a, b string) bool {
	return a < b
}

// Slices.
func _(a []string, b string, c [][]string) {
	lo.MinBy(a, less)                    // want `lo.MinBy can be replaced with slices.MinFunc`
	lo.MaxBy(a, less)                    // want `lo.MaxBy can be replaced with slices.MaxFunc`
	lo.MaxBy(a, func(x, y string) bool { // want `lo.MaxBy can be replaced with slices.MaxFunc`
		return less(x, y)
	})
	lo.MaxBy(a, func(x, y string) bool { // want `lo.MaxBy can be replaced with slices.MaxFunc`
		return x == y
	})
}
