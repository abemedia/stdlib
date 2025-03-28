package lo

func CoalesceOrEmpty[T comparable](v ...T) T {
	return v[0]
}
