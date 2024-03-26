//go:build debug

package ergo

import (
	"net/http"
	_ "net/http/pprof"
)

func init() {
	// start profiler
	dsn := "localhost:9009"
	go http.ListenAndServe(dsn, nil)
}
