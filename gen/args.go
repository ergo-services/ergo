package gen

// Arg returns the argument i as T; false: no argument i, nil or another type.
func Arg[T any](args []any, i int) (T, bool) {
	var zero T
	if i < 0 || i >= len(args) {
		return zero, false
	}
	v, ok := args[i].(T)
	return v, ok
}
