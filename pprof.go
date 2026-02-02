//go:build pprof

package ergo

import (
	"net/http"
	_ "net/http/pprof"
	"os"
)

func init() {
	host := os.Getenv("PPROF_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("PPROF_PORT")
	if port == "" {
		port = "9009"
	}
	go http.ListenAndServe(host+":"+port, nil)
}
