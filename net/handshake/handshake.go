package handshake

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"sync"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/edf"
)

type handshake struct {
	poolsize     int // how many TCP links accepts withing the connection
	flags        gen.NetworkFlags
	atom_mapping map[gen.Atom]gen.Atom
}

type Options struct {
	PoolSize     int
	NetworkFlags gen.NetworkFlags
	AtomMapping  map[gen.Atom]gen.Atom
}

func Create(options Options) gen.NetworkHandshake {
	var mapping map[gen.Atom]gen.Atom
	if options.PoolSize < 1 {
		options.PoolSize = defaultPoolSize
	}
	if len(options.AtomMapping) > 0 {
		mapping = make(map[gen.Atom]gen.Atom)
		for k, v := range options.AtomMapping {
			mapping[k] = v
		}
	}
	return &handshake{
		poolsize:     options.PoolSize,
		flags:        options.NetworkFlags,
		atom_mapping: mapping,
	}
}

func (h *handshake) NetworkFlags() gen.NetworkFlags {
	return h.flags
}

func (h *handshake) Version() gen.Version {
	return gen.Version{
		Name:    handshakeName,
		Release: handshakeRelease,
		License: gen.LicenseMIT,
	}
}

func (h *handshake) writeMessage(conn net.Conn, message any) error {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	buf.Allocate(6)
	buf.B[0] = handshakeMagic
	buf.B[1] = handshakeVersion

	if err := edf.Encode(message, buf, edf.Options{}); err != nil {
		return err
	}

	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()-6))
	l := buf.Len()
	lenP := l
	for {
		n, e := conn.Write(buf.B[lenP-l:])
		if e != nil {
			return e
		}
		// check if something left
		l -= n
		if l == 0 {
			break
		}
	}
	return nil
}

func (h *handshake) readMessage(conn net.Conn, timeout time.Duration, chunk []byte) (any, []byte, error) {
	var b [4096]byte

	if timeout == 0 {
		conn.SetReadDeadline(time.Time{})
	}

	expect := 6
	for {
		if len(chunk) < expect {
			if timeout > 0 {
				conn.SetReadDeadline(time.Now().Add(timeout))
			}

			n, err := conn.Read(b[:])
			if err != nil {
				return nil, nil, err
			}

			chunk = append(chunk, b[:n]...)
			continue
		}

		if chunk[0] != handshakeMagic {
			return nil, nil, fmt.Errorf("malformed handshake packet")
		}
		if chunk[1] != handshakeVersion {
			return nil, nil, fmt.Errorf("mismatch handshake version")
		}

		l := int(binary.BigEndian.Uint32(chunk[2:6]))
		if l > math.MaxUint16 {
			return nil, nil, fmt.Errorf("too long handshake message")
		}

		if len(chunk) < 6+l {
			expect = 6 + l
			continue
		}

		return edf.Decode(chunk[6:], edf.Options{})
	}
}

func (h *handshake) makeEncodeAtomCache(local map[uint16]gen.Atom) *sync.Map {
	if len(local) == 0 {
		return nil
	}
	cache := new(sync.Map)
	for k, v := range local {
		cache.Store(v, k)
	}
	return cache
}

func (h *handshake) makeEncodeRegCache(local map[uint16]string) *sync.Map {
	var names []string
	for _, name := range local {
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil
	}
	return edf.MakeEncodeRegTypeCache(names)
}

func (h *handshake) makeEncodeErrCache(local map[uint16]error) *sync.Map {
	if len(local) == 0 {
		return nil
	}
	cache := new(sync.Map)
	for k, v := range local {
		cache.Store(v, k)
	}
	return cache
}

func (h *handshake) makeDecodeAtomCache(remote map[uint16]gen.Atom) *sync.Map {
	if len(remote) == 0 {
		return nil
	}
	cache := new(sync.Map)
	for k, v := range remote {
		cache.Store(k, v)
	}
	return cache
}

func (h *handshake) makeDecodeRegCache(remote map[uint16]string) *sync.Map {
	if len(remote) == 0 {
		return nil
	}
	cache := new(sync.Map)
	for k, v := range remote {
		cache.Store(k, v)
	}
	return cache
}

func (h *handshake) makeDecodeErrCache(local, remote map[uint16]error) *sync.Map {
	if len(remote) == 0 {
		return nil
	}
	c := new(sync.Map)
	localRegisteredErrors := make(map[string]error)
	for _, v := range local {
		localRegisteredErrors[v.Error()] = v
	}
	for k, v := range remote {
		if err, exist := localRegisteredErrors[v.Error()]; exist {
			c.Store(k, err)
			continue
		}
		c.Store(k, v)
	}
	return c
}
