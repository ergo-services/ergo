package proto

import (
	"sync"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/net/edf"
	"ergo.services/ergo/net/handshake"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

// applyCacheUpdate copies the peer's cache entries into the local decode caches.
func TestApplyCacheUpdate(t *testing.T) {
	c := &connection{
		log:           mock.NewLog(),
		encodeOptions: edf.Options{Cache: new(sync.Map)},
		decodeOptions: edf.Options{
			AtomCache:   new(sync.Map),
			AtomMapping: new(sync.Map),
			RegCache:    new(sync.Map),
			ErrCache:    new(sync.Map),
		},
	}

	c.applyCacheUpdate(MessageUpdateCache{
		AtomCache:   map[uint16]gen.Atom{7: "worker"},
		AtomMapping: map[gen.Atom]gen.Atom{"short": "long"},
		RegCache:    map[uint16]string{3: "pkg.Type"},
		ErrCache:    map[uint16]error{5: gen.ErrProcessUnknown},
	})

	atom, ok := c.decodeOptions.AtomCache.Load(uint16(7))
	check.True(t, ok)
	check.Equal(t, gen.Atom("worker"), atom)

	mapped, ok := c.decodeOptions.AtomMapping.Load(gen.Atom("short"))
	check.True(t, ok)
	check.Equal(t, gen.Atom("long"), mapped)

	reg, ok := c.decodeOptions.RegCache.Load(uint16(3))
	check.True(t, ok)
	check.Equal(t, "pkg.Type", reg)

	cerr, ok := c.decodeOptions.ErrCache.Load(uint16(5))
	check.True(t, ok)
	check.Equal(t, gen.ErrProcessUnknown, cerr)
}

// NewConnection must leave the decode caches non-nil even when the peer advertised
// empty caches, so a later MessageUpdateCache cannot LoadOrStore into a nil map.
func TestNewConnectionDecodeCachesNonNil(t *testing.T) {
	result := gen.HandshakeResult{
		ConnectionID: "conn-1",
		Peer:         "peer@localhost",
		PeerCreation: 1,
		PoolSize:     1,
		Custom:       handshake.ConnectionOptions{PoolSize: 1}, // nil Decode* caches
	}
	conn, err := (&enp{}).NewConnection(mock.NewCore(), result, mock.NewLog())
	if err != nil {
		t.Fatal(err)
	}
	c := conn.(*connection)

	check.True(t, c.decodeOptions.AtomCache != nil)
	check.True(t, c.decodeOptions.AtomMapping != nil)
	check.True(t, c.decodeOptions.RegCache != nil)
	check.True(t, c.decodeOptions.ErrCache != nil)

	// the crash site: a peer's cache update must LoadOrStore without a nil-map panic
	c.applyCacheUpdate(MessageUpdateCache{
		AtomCache: map[uint16]gen.Atom{7: "worker"},
		RegCache:  map[uint16]string{3: "pkg.Type"},
		ErrCache:  map[uint16]error{5: gen.ErrProcessUnknown},
	})
	atom, ok := c.decodeOptions.AtomCache.Load(uint16(7))
	check.True(t, ok)
	check.Equal(t, gen.Atom("worker"), atom)
}
