package registrar

import (
	"time"

	"ergo.services/ergo/gen"
)

const (
	registrarName    string = "ESRD" // Ergo Service Registration and Discovery
	registrarRelease string = "R1"   // (Rev.1)

	defaultRegistrarPort uint16        = 4499
	defaultKeepAlive     time.Duration = 3 * time.Second

	// initial registration retries transient network errors (owner dying/promotion
	// window) for this long before failing the node start. registration rejections
	// (gen.ErrTaken and the like) are not retried.
	registerRetryTimeout  time.Duration = 3 * time.Second
	registerRetryInterval time.Duration = 50 * time.Millisecond

	protoVersion       byte = 1
	protoRegister      byte = 44
	protoRegisterReply byte = 45
	protoResolve       byte = 46
	protoResolveReply  byte = 47
)

type MessageRegisterRoutes struct {
	Node   gen.Atom
	Routes []gen.Route
}

type MessageRegisterReply struct {
	Error error
}

type MessageResolveRoutes struct {
	Node gen.Atom
}

type MessageResolveReply struct {
	Routes []gen.Route
	Error  error
}
