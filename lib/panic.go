package lib

import (
	"fmt"
	"runtime"
	"strings"
)

// PanicOrigin returns "function[file:line]" a panic came from. Call it from the deferred recover.
func PanicOrigin() string {
	var pcs [32]uintptr
	n := runtime.Callers(3, pcs[:])
	if n == 0 {
		return "unknown"
	}

	frames := runtime.CallersFrames(pcs[:n])
	origin, more := frames.Next()
	for strings.HasPrefix(origin.Function, "runtime.") && more {
		origin, more = frames.Next()
	}

	return fmt.Sprintf("%s[%s:%d]", origin.Function, origin.File, origin.Line)
}
