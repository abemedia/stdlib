package test

import (
	"golang.org/x/exp/maps"     // want "Package \"golang.org/x/exp/maps\" can be replaced with \"maps\""
	"golang.org/x/exp/rand"     // want "Package \"golang.org/x/exp/rand\" can be replaced with \"math/rand/v2\""
	"golang.org/x/exp/slices"   // want "Package \"golang.org/x/exp/slices\" can be replaced with \"slices\""
	"golang.org/x/exp/slog"     // want "Package \"golang.org/x/exp/slog\" can be replaced with \"log/slog\""
	"golang.org/x/net/context"  // want "Package \"golang.org/x/net/context\" can be replaced with \"context\""
	"golang.org/x/sync/syncmap" // want "Package \"golang.org/x/sync/syncmap\" can be replaced with \"sync\""
)

func _(a map[string]int, b []string) {
	rand.New(rand.NewSource(1))
	maps.Keys(a)
	slices.Clone(b)
	slog.Error("test")
	context.Background()
	_ = syncmap.Map{}
}
