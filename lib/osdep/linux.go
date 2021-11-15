//go:build linux
// +build linux

package osdep

import (
	"syscall"
)

// ResourceUsage
func ResourceUsage() (int64, int64) {
	var usage syscall.Rusage
	var utime, stime int64
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err == nil {
		utime = usage.Utime.Sec*1000000000 + usage.Utime.Nano()
		stime = usage.Stime.Sec*1000000000 + usage.Stime.Nano()
	}
	return utime, stime
}
