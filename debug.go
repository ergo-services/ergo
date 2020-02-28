//+build debug

package ergo

import (
	"net/http"
	_ "net/http/pprof"
)

func init() {
	go http.ListenAndServe("0.0.0.0:9009", nil)
}
