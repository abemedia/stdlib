package test

import (
	. "golang.org/x/exp/rand"     // want "Package \"golang.org/x/exp/rand\" can be replaced with \"math/rand/v2\""
	. "golang.org/x/sync/syncmap" // want "Package \"golang.org/x/sync/syncmap\" can be replaced with \"sync\""
)

func _(a map[string]int, b []string) {
	New(NewSource(1))
	_ = Map{}
}
