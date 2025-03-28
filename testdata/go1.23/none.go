package test

import (
	"test/lo"
)

func _(a, b string) {
	lo.CoalesceOrEmpty(a, b)
}
