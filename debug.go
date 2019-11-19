//+build debug

package ergonode

import (
	_ "expvar"
	"net/http"
	_ "net/http/pprof"
)

func init() {
	go http.ListenAndServe("0.0.0.0:9009", nil)
}
