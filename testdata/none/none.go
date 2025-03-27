package none

import (
	"none/lo"
)

func _(a, b string) {
	lo.CoalesceOrEmpty(a, b)
}
