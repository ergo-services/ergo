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
			AtomMapping: opts.EncodeAtomMapping,
			AtomCache:   opts.EncodeAtomCache,
			RegCache:    opts.EncodeRegCache,
			ErrCache:    opts.EncodeErrCache,
			Cache:       new(sync.Map),
		},

		decodeOptions: edf.Options{
			AtomMapping: opts.DecodeAtomMapping,
			AtomCache:   opts.DecodeAtomCache,
			RegCache:    opts.DecodeRegCache,
			ErrCache:    opts.DecodeErrCache,
			Cache:       new(sync.Map),
		},
		requests: make(map[gen.Ref]chan MessageResult),
	}

	// init recv queues. create 4 recv queues per connection
	// since the decoding is more costly comparing to the encoding
	for i := 0; i < opts.PoolSize*4; i++ {
		conn.recvQueues = append(conn.recvQueues, lib.NewQueueMPSC())
	}

	return conn, nil
}

func (e *enp) Serve(c gen.Connection, dial gen.NetworkDial) error {
	conn := c.(*connection)
	if dial == nil {
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
		conn.log.Trace("dialing %s (pool: %d of %d)", dsn, i+1, conn.pool_size)
		nc, tail, err := dial(dsn, conn.id)
		if err != nil {
			conn.log.Trace("dialing %s failed: %s", dsn, err)
			continue
		}

		if err := conn.Join(nc, conn.id, dial, tail); err != nil {
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
