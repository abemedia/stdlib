package slices

import (
	"github.com/samber/lo" // want "The github.com/samber/lo package import is no longer necessary"
)

// Slices.
func _(a []string, b string, c [][]string) {
	lo.Chunk(a, 2)                         // want `lo.Chunk can be replaced with slices.Chunk`
	lo.Drop(a, 2)                          // want `lo.Drop can be replaced with builtin`
	lo.DropRight(a, 2)                     // want `lo.DropRight can be replaced with builtin`
	lo.Contains(a, b)                      // want `lo.Contains can be replaced with slices.Contains`
	lo.ContainsBy(a, func(s string) bool { // want `lo.ContainsBy can be replaced with slices.ContainsFunc`
		return s == b
	})
	lo.IndexOf(a, b)                     // want `lo.IndexOf can be replaced with slices.Index`
	lo.LastIndexOf(a, b)                 // want `lo.LastIndexOf can be replaced with slices.LastIndex`
	lo.Min(a)                            // want `lo.Min can be replaced with slices.Min`
	lo.MinBy(a, func(x, y string) bool { // want `lo.MinBy can be replaced with slices.MinFunc`
		return len(x) < len(y)
	})
	lo.MinBy(a, func(x, y string) bool { // want `lo.MinBy can be replaced with slices.MinFunc`
		return x > y
	})
	lo.Max(a)                            // want `lo.Max can be replaced with slices.Max`
	lo.MaxBy(a, func(x, y string) bool { // want `lo.MaxBy can be replaced with slices.MaxFunc`
		if b == "" {
			return x > y
		}
		return x < y
	})
	lo.MaxBy(a, func(x, y string) bool { // want `lo.MaxBy can be replaced with slices.MaxFunc`
		return x > y
	})
	lo.IsSorted(a) // want `lo.IsSorted can be replaced with slices.IsSorted`
	lo.Flatten(c)  // want `lo.Flatten can be replaced with slices.Concat`
}

// Maps.
func _(a map[string]string, b string) {
	lo.Keys(a)   // want `lo.Keys can be replaced with maps.Keys`
	lo.Values(a) // want `lo.Values can be replaced with maps.Values`
}

// Helpers.
func _(a, b string) {
	lo.CoalesceOrEmpty(a, b) // want `lo.CoalesceOrEmpty can be replaced with cmp.Or`
}
