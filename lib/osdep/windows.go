// +build windows

package osdep

func getResourceUsage() (int64, int64) {
	// FIXME Windows doesn't support syscall.Rusage. There is should be another
	// way to get this kind of data from the OS
	return 0, 0
}
