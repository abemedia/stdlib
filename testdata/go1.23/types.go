package test

import (
	"golang.org/x/exp/constraints" // want "The golang.org/x/exp/constraints package import is no longer necessary"
)

type _ interface {
	constraints.Ordered // want "golang.org/x/exp/constraints.Ordered can be replaced with cmp.Ordered"
}

func _[T constraints.Ordered](v T) {} // want "golang.org/x/exp/constraints.Ordered can be replaced with cmp.Ordered"
