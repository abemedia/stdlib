package test

import (
	xslices "golang.org/x/exp/slices" // want "Package \"golang.org/x/exp/slices\" can be replaced with \"slices\""

	"github.com/samber/lo"
	hi "github.com/samber/lo" // want "The github.com/samber/lo package import is no longer necessary"
)

func _(a []string) {
	lo.Keyify(a)
	hi.Drop(a, 2)      // want `lo.Drop can be replaced with builtin`
	hi.DropRight(a, 2) // want `lo.DropRight can be replaced with builtin`
	xslices.Clone(a)
}
