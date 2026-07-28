// Package proto implements ENP, the default Ergo network protocol: EDF message encoding,
// the wire framing, and the multi-TCP connection to a remote node.
//
// # Connection model
//
// A peer connection is one logical link to a remote node carried over a pool of TCP
// connections:
//
//   - primary: the first TCP, brought up by the full handshake; the pool's first member.
//   - pool, pool_size: the primary plus extra TCPs, filled to pool_size (the acceptor's
//     configured size, advertised in the handshake). Several TCPs give parallel delivery.
//   - ConnectionID: a direction-independent id from the handshake; both directions of a
//     pair compute the same one, so it identifies the connection regardless of who dialed.
//
// # Establishment and the connect storm
//
// Two nodes may dial each other at the same instant (a connect storm, normal while a full
// mesh forms): both directions finish the handshake, then a merge keyed on ConnectionID
// keeps one connection per pair. The merge is decided by the node (node/network.go); this
// package carries it out and fills the pool.
//
//   - canonical: the node with the smaller name in a pair. The canonical direction (the
//     dial from the smaller name) is the merge survivor, and the canonical end is the
//     default pool filler.
//   - fill, go-ahead: the dialer fills the pool, having reached the acceptor listener. A
//     canonical dialer fills at once; a non-canonical dialer fills only after the acceptor
//     sends it a go-ahead (protoMessageExtend), so it never fills a connection that a
//     simultaneous connect is about to supersede.
package proto

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/edf"
	"ergo.services/ergo/net/handshake"
)

type enp struct {
	core gen.Core
}

func Create() gen.NetworkProto {
	return &enp{}
}

// gen.NetworkProto implementation

func (e *enp) NewConnection(core gen.Core, result gen.HandshakeResult, log gen.Log) (gen.Connection, error) {

	opts, ok := result.Custom.(handshake.ConnectionOptions)
	if ok == false {
		return nil, fmt.Errorf("connection with %s: HandshakeResult.Custom has unexpected type %T, want handshake.ConnectionOptions", result.Peer, result.Custom)
	}

	if opts.PoolSize < 1 {
		opts.PoolSize = 1
	}
	if opts.PoolSize > gen.DefaultMaxConnectionPoolSize {
		opts.PoolSize = gen.DefaultMaxConnectionPoolSize
	}

	if result.PeerCreation == 0 {
		// seems it was Join handshake for the connection that was already terminated
		return nil, gen.ErrNotAllowed
	}

	log.Trace("create new connection with %s (pool size: %d)", result.Peer, opts.PoolSize)
	conn := &connection{
		id:                  result.ConnectionID,
		creation:            time.Now().Unix(),
		core:                core,
		log:                 log,
		node_flags:          result.NodeFlags,
		node_maxmessagesize: result.NodeMaxMessageSize,

		handshakeVersion: result.HandshakeVersion,
		protoVersion:     e.Version(),

		peer:                result.Peer,
		peer_creation:       result.PeerCreation,
		peer_flags:          result.PeerFlags,
		peer_version:        result.PeerVersion,
		peer_maxmessagesize: result.PeerMaxMessageSize,

		pool_size: opts.PoolSize,
		pool_dsn:  opts.PoolDSN,
		tls:       opts.TLS,

		encodeOptions: edf.Options{
			AtomCache: opts.EncodeAtomCache,
			RegCache:  opts.EncodeRegCache,
			ErrCache:  opts.EncodeErrCache,
			Cache:     new(sync.Map),
			WrappedErrorsSupported: result.NodeFlags.EnableWrappedErrors &&
				result.PeerFlags.EnableWrappedErrors,
			SchemaEvolution: result.NodeFlags.EnableSchemaEvolution &&
				result.PeerFlags.EnableSchemaEvolution,
		},

		decodeOptions: edf.Options{
			AtomCache: opts.DecodeAtomCache,
			RegCache:  opts.DecodeRegCache,
			ErrCache:  opts.DecodeErrCache,
			Cache:     new(sync.Map),
			WrappedErrorsSupported: result.NodeFlags.EnableWrappedErrors &&
				result.PeerFlags.EnableWrappedErrors,
			SchemaEvolution: result.NodeFlags.EnableSchemaEvolution &&
				result.PeerFlags.EnableSchemaEvolution,
		},
		requests: make(map[gen.Ref]chan MessageResult),

		softwareKeepAlive: result.NodeFlags.EnableSoftwareKeepAlive > 0 &&
			result.PeerFlags.EnableSoftwareKeepAlive > 0,
	}
	conn.done = make(chan struct{})

	if conn.softwareKeepAlive {
		myPeriod := time.Duration(result.NodeFlags.EnableSoftwareKeepAlive) * time.Second
		peerPeriod := time.Duration(result.PeerFlags.EnableSoftwareKeepAlive) * time.Second
		misses := opts.SoftwareKeepAliveMisses
		if misses == 0 {
			misses = gen.DefaultSoftwareKeepAliveMisses
		}
		conn.softwareKeepAlivePeriod = myPeriod
		conn.softwareKeepAliveMisses = misses
		conn.softwareKeepAliveTimeout = peerPeriod * time.Duration(misses)
		conn.softwareKeepAliveMessage = []byte{
			protoMagic, protoVersion, 0, 0, 0, 8, 0, protoMessageK,
		}
	}

	if result.NodeFlags.EnableClockSkew == true &&
		result.PeerFlags.EnableClockSkew == true {
		conn.clockSkew = true
	}

	if result.NodeFlags.EnableTracing == true &&
		result.PeerFlags.EnableTracing == true {
		conn.tracing = true
	}

	if result.NodeFlags.EnableFragmentation == true &&
		result.PeerFlags.EnableFragmentation == true {
		conn.fragmentation = true
		conn.fragmentSize = gen.DefaultFragmentSize
		if opts.FragmentSize > 0 {
			conn.fragmentSize = opts.FragmentSize
		}
		conn.fragmentTimeout = gen.DefaultFragmentTimeout
		if opts.FragmentTimeout > 0 {
			conn.fragmentTimeout = time.Duration(opts.FragmentTimeout) * time.Second
		}
		conn.maxFragmentAssemblies = gen.DefaultMaxFragmentAssemblies
		if opts.MaxFragmentAssemblies > 0 {
			conn.maxFragmentAssemblies = opts.MaxFragmentAssemblies
		}
		conn.sharedFragments = make(map[uint32]*fragmentAssembly)
		conn.sharedFragTimer = time.AfterFunc(time.Hour, conn.cleanupSharedFragments)
		conn.sharedFragTimer.Stop()
	}

	if len(result.AtomMapping) > 0 {
		conn.encodeOptions.AtomMapping = &sync.Map{}
		conn.decodeOptions.AtomMapping = &sync.Map{}
		for k, v := range result.AtomMapping {
			conn.encodeOptions.AtomMapping.Store(k, v)
			conn.decodeOptions.AtomMapping.Store(v, k)
		}
	}

	// decode caches must be non-nil so a peer's MessageUpdateCache can LoadOrStore into them
	if conn.decodeOptions.AtomCache == nil {
		conn.decodeOptions.AtomCache = &sync.Map{}
	}
	if conn.decodeOptions.AtomMapping == nil {
		conn.decodeOptions.AtomMapping = &sync.Map{}
	}
	if conn.decodeOptions.RegCache == nil {
		conn.decodeOptions.RegCache = &sync.Map{}
	}
	if conn.decodeOptions.ErrCache == nil {
		conn.decodeOptions.ErrCache = &sync.Map{}
	}

	// init recv queues. create 4 recv queues per connection
	// since the decoding is more costly comparing to the encoding
	numQueues := opts.PoolSize * 4
	for i := 0; i < numQueues; i++ {
		conn.recvQueues = append(conn.recvQueues, lib.NewQueueMPSC())
	}

	// init route queues for protoMessageAny dispatch. These drain Link/Monitor/
	// Spawn/etc so decoding goroutines never block on synchronous TM/core work.
	for i := range conn.routeQueues {
		conn.routeQueues[i] = lib.NewQueueMPSC()
	}

	// init per-queue ordered fragment assembly maps
	if conn.fragmentation {
		conn.orderedFragments = make([]map[uint32]*fragmentAssembly, numQueues)
		for i := 0; i < numQueues; i++ {
			conn.orderedFragments[i] = make(map[uint32]*fragmentAssembly)
		}
	}

	return conn, nil
}

func (e *enp) Serve(c gen.Connection, redial gen.NetworkDial) error {
	conn := c.(*connection)
	// the dialer reached the peer's listener, so it is the pool filler (redial != nil); the
	// accept side (redial == nil) stays passive, as does a connection too small to pool
	// (pool_size < 2) or one with no pool DSN to dial.
	if redial == nil || conn.pool_size < 2 || len(conn.pool_dsn) == 0 {
		conn.wait()
		return nil
	}

	// a canonical-direction dialer always survives the merge, so it fills now. A
	// non-canonical dialer may be a storm loser, so it fills only when the acceptor
	// confirms it is the survivor via protoMessageExtend (handled in serve()); until then
	// it stays passive so it never fills a connection a simultaneous connect supersedes.
	conn.pool_mutex.Lock()
	conn.redial = redial
	fillNow := conn.core.Name() < conn.peer || conn.extendRequested
	conn.pool_mutex.Unlock()
	if fillNow {
		conn.startFill()
	}

	conn.wait()
	return nil
}

func (e *enp) Version() gen.Version {
	return gen.Version{
		Name:    protoName,
		Release: protoRelease,
		License: gen.LicenseMIT,
	}
}

// gen.TypeRegistry implementation (wire-format type registry on top of edf).

func (e *enp) RegisterType(v any) error        { return edf.RegisterTypeOf(v) }
func (e *enp) RegisterTypes(types []any) error { return edf.RegisterTypesOf(types) }
func (e *enp) RegisterError(err error) error   { return edf.RegisterError(err) }
func (e *enp) RegisterAtom(a gen.Atom) error   { return edf.RegisterAtom(a) }

func (e *enp) RegisterErrors(errs []error) error {
	for _, err := range errs {
		if e := edf.RegisterError(err); e != nil && e != gen.ErrTaken {
			return e
		}
	}
	return nil
}

func (e *enp) RegisterAtoms(atoms []gen.Atom) error {
	for _, a := range atoms {
		if err := edf.RegisterAtom(a); err != nil && err != gen.ErrTaken {
			return err
		}
	}
	return nil
}

func (e *enp) RegisteredTypes() []gen.RegisteredTypeInfo {
	list := edf.RegisteredTypes()
	ver := e.Version().Str()
	for i := range list {
		list[i].Proto = ver
	}
	return list
}

func (e *enp) LookupType(name string) (reflect.Type, bool) {
	return edf.LookupType(name)
}
