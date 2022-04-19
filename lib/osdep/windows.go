//go:build windows
// +build windows

package osdep

// ResourceUsage
func ResourceUsage() (int64, int64) {
	// FIXME Windows doesn't support syscall.Rusage. There should be another
	// way to get this kind of data from the OS
	return 0, 0
}
