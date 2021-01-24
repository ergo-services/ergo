// +build windows

package ergo

func os_dep_getResourceUsage() (int64, int64) {
	// FIXME Windows doesn't support syscall.Rusage. There is should be another
	// way to get this kind of data from the OS
	return 0, 0
}
