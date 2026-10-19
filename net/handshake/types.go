package handshake

import (
	"reflect"
	"sync"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/net/edf"
)

const (
	handshakeName    string = "EHS"
	handshakeRelease string = "R1" // Ergo Handshake (Rev.1)

	handshakeMagic   byte = 87
	handshakeVersion byte = 1

	defaultPoolSize int = 3

	// handshakeMaxControlSize bounds a control message read (Hello, Join, Accept, Reject).
	// These are small and fixed-shape; the ceiling also caps pre-auth buffering on the first
	// (unauthenticated) read. The larger Introduce read uses the configurable node limit.
	handshakeMaxControlSize int = 64 * 1024
)

type MessageHello struct {
	Salt       string
	Digest     string
	DigestCert string
}

type MessageJoin struct {
	Node         gen.Atom
	ConnectionID string
	Salt         string
	Digest       string
}

type MessageIntroduce struct {
	Node     gen.Atom
	Version  gen.Version
	Flags    gen.NetworkFlags
	Creation int64

	MaxMessageSize int

	AtomCache map[uint16]gen.Atom
	RegCache  map[uint16]string
	ErrCache  map[uint16]error
	Digest    string
}

type MessageAccept struct {
	ID         string
	PoolSize   int
	PoolDSN    []string
	Digest     string
	DigestCert string
}

type MessageReject struct {
	Reason string
}

type ConnectionOptions struct {
	PoolSize int
	PoolDSN  []string
	TLS      bool

	// Registry is the node registry; Revision is the part of it the peer
	// learned in the handshake.
	Registry *edf.Registry
	Revision uint64

	DecodeAtomCache *sync.Map
	DecodeRegCache  *sync.Map
	DecodeErrCache  *sync.Map

	SoftwareKeepAliveMisses int
	FragmentSize            int
	FragmentTimeout         int
	MaxFragmentAssemblies   int

	// acceptor handshake continuation between Negotiate and Accept; opaque to proto
	pendingAccept    MessageAccept
	pendingIntroduce MessageIntroduce
	pendingTail      []byte
	awaitingAccept   bool
}

var (
	registry     *edf.Registry
	controlTypes = make(map[string]bool)
)

func init() {
	types := []any{
		MessageHello{},
		MessageJoin{},
		MessageIntroduce{},
		MessageAccept{},
		MessageReject{},
	}

	registry = edf.NewRegistry()
	for _, t := range types {
		rt := reflect.TypeOf(t)
		controlTypes["#"+rt.PkgPath()+"/"+rt.Name()] = true
		err := registry.RegisterType(t)
		if err == nil || err == gen.ErrTaken {
			continue
		}
		panic(err)
	}
}
