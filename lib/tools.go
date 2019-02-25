package lib

import (
	"flag"
	"log"
)

var nTrace bool

func init() {
	flag.BoolVar(&nTrace, "trace.node", false, "trace node")
}

func Log(f string, a ...interface{}) {
	if nTrace {
		log.Printf(f, a...)
	}
}
