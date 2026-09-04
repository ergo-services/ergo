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
	protoNodes         byte = 48
	protoNodesReply    byte = 49
	protoNodeEvent     byte = 50

	// nodesRequestTimeout bounds a nodes request to a peer host. A host with no
	// ESRD server simply never answers.
	nodesRequestTimeout time.Duration = time.Second

	// nodesCacheTTL is how long the result of Nodes() is reused. Every call
	// costs one UDP datagram per known peer host.
	nodesCacheTTL time.Duration = 3 * time.Second
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

type MessageNodes struct{}

type MessageNodesReply struct {
	Nodes []gen.Atom
}

// MessageNodeEvent is pushed by the server to its clients over the registration link.
type MessageNodeEvent struct {
	Node   gen.Atom
	Joined bool
}
