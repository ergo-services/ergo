package proto

import (
	"sync"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/net/edf"
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
