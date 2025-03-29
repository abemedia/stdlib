package test

import (
	. "github.com/samber/lo" // want "The github.com/samber/lo package import is no longer necessary"
)

func _(a []string) {
	Min(a) // want `lo.Min can be replaced with slices.Min`
}
