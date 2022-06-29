//go:build freebsd || openbsd || netbsd || dragonfly
// +build freebsd openbsd netbsd dragonfly

package osdep

import (
	"syscall"
)

// ResourceUsage
func ResourceUsage() (int64, int64) {
	var usage syscall.Rusage
	var utime, stime int64
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err == nil {
		utime = int64(usage.Utime.Sec)*1000000000 + usage.Utime.Nano()
		stime = int64(usage.Stime.Sec)*1000000000 + usage.Stime.Nano()
	}
	return utime, stime
}
