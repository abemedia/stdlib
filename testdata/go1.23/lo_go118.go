//go:build go1.18

package test

import (
	"github.com/samber/lo"
)

// Slices.
func _(a []string, b string, c [][]string) {
	lo.Chunk(a, 2)
	lo.Drop(a, 2)      // want `lo.Drop can be replaced with builtin`
	lo.DropRight(a, 2) // want `lo.DropRight can be replaced with builtin`
	lo.Contains(a, b)
	lo.ContainsBy(a, func(s string) bool {
		return s == b
	})
	lo.IndexOf(a, b)
	lo.Min(a)
	lo.MinBy(a, func(x, y string) bool {
		return x > y
	})
	lo.Max(a)
	lo.MaxBy(a, func(x, y string) bool {
		return x > y
	})
	lo.IsSorted(a)
	lo.Flatten(c)
}

// Maps.
func _(a map[string]string, b string) {
	lo.Keys(a)
	lo.Values(a)
}

// Helpers.
func _(a, b string) {
	lo.CoalesceOrEmpty(a, b)
}
