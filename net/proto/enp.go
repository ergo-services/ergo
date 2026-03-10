package proto

import (
	"fmt"
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
		return nil, fmt.Errorf("HandshakeResult.Custom has unknown type")
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

		encodeOptions: edf.Options{
			AtomCache: opts.EncodeAtomCache,
			RegCache:  opts.EncodeRegCache,
			ErrCache:  opts.EncodeErrCache,
			Cache:     new(sync.Map),
		},

		decodeOptions: edf.Options{
			AtomCache: opts.DecodeAtomCache,
			RegCache:  opts.DecodeRegCache,
			ErrCache:  opts.DecodeErrCache,
			Cache:     new(sync.Map),
		},
		requests: make(map[gen.Ref]chan MessageResult),

		softwareKeepAlive: result.NodeFlags.EnableSoftwareKeepAlive > 0 &&
			result.PeerFlags.EnableSoftwareKeepAlive > 0,
	}

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
			protoMagic, protoVersion, 0, 0, 0, 8, 0, protoMessageSoftwareKeepAlive,
		}
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

	// init recv queues. create 4 recv queues per connection
	// since the decoding is more costly comparing to the encoding
	numQueues := opts.PoolSize * 4
	for i := 0; i < numQueues; i++ {
		conn.recvQueues = append(conn.recvQueues, lib.NewQueueMPSC())
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
	if redial == nil {
		// accepted connection. no dialer.
		conn.wait()
		return nil
	}

	if conn.pool_size < 2 {
		// just one TCP connection in the pool
		conn.wait()
		return nil
	}

	if len(conn.pool_dsn) == 0 {
		conn.log.Warning("pool size is %d, but DSN list is empty", conn.pool_size)
		conn.wait()
		return nil
	}

	for i := 1; i < conn.pool_size; i++ {

		// TODO
		// we should try the next dsn on dialing failure

		n := i % len(conn.pool_dsn)
		dsn := conn.pool_dsn[n]
		if lib.Trace() {
			conn.log.Trace("dialing %s (pool: %d of %d)", dsn, i+1, conn.pool_size)
		}
		nc, tail, err := redial(dsn, conn.id)
		if err != nil {
			if lib.Trace() {
				conn.log.Trace("dialing %s failed: %s", dsn, err)
			}
			continue
		}

		if err := conn.Join(nc, conn.id, redial, tail); err != nil {
			conn.log.Error("unable to join %s: %s", nc.RemoteAddr().String(), err)
		}
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
