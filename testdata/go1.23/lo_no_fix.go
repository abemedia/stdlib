package test

import (
	"github.com/samber/lo"
)

func less(a, b string) bool {
	return a < b
}

func key(a string) int {
	return len(a)
}

// Slices.
func _(a []string, b string, c [][]string) {
	lo.MinBy(a, less)                         // want `lo.MinBy can be replaced with slices.MinFunc`
	lo.MinBy(a, func(x, y string) (ok bool) { // want `lo.MinBy can be replaced with slices.MinFunc`
		return
	})
	lo.MaxBy(a, less)                    // want `lo.MaxBy can be replaced with slices.MaxFunc`
	lo.MaxBy(a, func(x, y string) bool { // want `lo.MaxBy can be replaced with slices.MaxFunc`
		return less(x, y)
	})
	lo.MaxBy(a, func(x, y string) bool { // want `lo.MaxBy can be replaced with slices.MaxFunc`
		return x == y
	})
	lo.IsSortedByKey(a, key)               // want `lo.IsSortedByKey can be replaced with slices.IsSortedFunc`
	lo.IsSortedByKey(a, func(string) int { // want `lo.IsSortedByKey can be replaced with slices.IsSortedFunc`
		return 1
	})
	lo.IsSortedByKey(a, func(x string) (y int) { // want `lo.IsSortedByKey can be replaced with slices.IsSortedFunc`
		return
	})
	lo.IsSortedByKey(a, func(x string) string { // want `lo.IsSortedByKey can be replaced with slices.IsSortedFunc`
		return b
	})
}
