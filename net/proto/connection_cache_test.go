package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/edf"
	"ergo.services/ergo/net/handshake"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

type cacheTestType struct{ A int }

var errCacheTest = errors.New("cache test sentinel")

// cacheConn is a receiving connection over its own registry.
func cacheConn(r *edf.Registry, log gen.Log) *connection {
	return &connection{
		peer:     "peer@localhost",
		log:      log,
		registry: r,
		decodeOptions: edf.Options{
			Registry:  r,
			AtomCache: new(sync.Map),
			RegCache:  new(sync.Map),
			ErrCache:  new(sync.Map),
		},
	}
}

func installFrames(t *testing.T, c *connection, frames [][]byte) {
	t.Helper()
	for _, f := range frames {
		check.Equal(t, protoMessageC, f[7])
		check.Equal(t, len(f), int(binary.BigEndian.Uint32(f[2:6])))
		check.NoError(t, c.installCache(f[8:]))
	}
}

// the entries published after a revision reach the receiver decode caches; a
// known error maps to the local sentinel.
func TestCacheFramesInstall(t *testing.T) {
	sender := edf.NewRegistry()
	snapshot := sender.Revision()
	check.NoError(t, sender.RegisterType(cacheTestType{}))
	check.NoError(t, sender.RegisterAtom("cache.atom"))
	check.NoError(t, sender.RegisterError(errCacheTest))
	d := sender.Since(snapshot)

	receiver := edf.NewRegistry()
	check.NoError(t, receiver.RegisterType(cacheTestType{}))
	check.NoError(t, receiver.RegisterError(errCacheTest))
	c := cacheConn(receiver, mock.NewLog())
	installFrames(t, c, cacheFrames(d))

	for id, name := range d.Types {
		v, ok := c.decodeOptions.RegCache.Load(id)
		check.True(t, ok)
		check.Equal(t, name, v.(string))
	}
	for id, atom := range d.Atoms {
		v, ok := c.decodeOptions.AtomCache.Load(id)
		check.True(t, ok)
		check.Equal(t, atom, v.(gen.Atom))
	}
	for id := range d.Errors {
		v, ok := c.decodeOptions.ErrCache.Load(id)
		check.True(t, ok)
		check.True(t, v.(edf.RemoteError).Value() == errCacheTest)
	}

	// the same frames again are a no-op
	installFrames(t, c, cacheFrames(d))
}

// an error installed before its local sentinel is registered resolves to the
// sentinel once it is.
func TestCacheInstallLateSentinel(t *testing.T) {
	late := errors.New("cache late sentinel")
	receiver := edf.NewRegistry()
	c := cacheConn(receiver, mock.NewLog())
	installFrames(t, c, cacheFrames(edf.Dictionary{Errors: map[uint16]error{40000: errors.New(late.Error())}}))

	v, ok := c.decodeOptions.ErrCache.Load(uint16(40000))
	check.True(t, ok)
	check.True(t, v.(edf.RemoteError).Value() != late)

	check.NoError(t, receiver.RegisterError(late))
	check.True(t, v.(edf.RemoteError).Value() == late)
}

// a frame with a conflicting entry is refused whole.
func TestCacheInstallConflict(t *testing.T) {
	c := cacheConn(edf.NewRegistry(), mock.NewLog())
	c.decodeOptions.RegCache.Store(uint16(5000), "#pkg/Taken")

	d := edf.Dictionary{
		Types: map[uint16]string{5000: "#pkg/Other"},
		Atoms: map[uint16]gen.Atom{300: "cache.conflict"},
	}
	frames := cacheFrames(d)
	check.Equal(t, 1, len(frames))
	check.Error(t, c.installCache(frames[0][8:]))
	_, installed := c.decodeOptions.AtomCache.Load(uint16(300))
	check.False(t, installed)

	for _, body := range [][]byte{
		{cacheKindType, 0x00, 0x10, 0x00, 0x01, 'x'}, // id in the length range
		{cacheKindAtom, 0x01, 0x2c, 0x00, 0x05, 'x'}, // truncated
		{9, 0x01, 0x2c, 0x00, 0x00},                  // unknown kind
	} {
		check.Error(t, c.installCache(body))
	}
}

// unknown types and errors produce one warning line each per install.
func TestCacheInstallWarnings(t *testing.T) {
	var warnings []string
	log := mock.NewLog()
	log.OnWarning(func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	})
	c := cacheConn(edf.NewRegistry(), log)
	installFrames(t, c, cacheFrames(edf.Dictionary{
		Types:  map[uint16]string{5000: "#pkg/A", 5001: "#pkg/B"},
		Errors: map[uint16]error{40000: errors.New("remote only")},
	}))
	check.Equal(t, 2, len(warnings))
	check.True(t, strings.Contains(warnings[0], "#pkg/A, #pkg/B"))
	check.True(t, strings.Contains(warnings[1], `"remote only"`))
}

// a large dictionary goes in several frames, each installable on its own.
func TestCacheFramesSplit(t *testing.T) {
	d := edf.Dictionary{Types: map[uint16]string{}}
	for i := 0; i < 2000; i++ {
		d.Types[uint16(5000+i)] = fmt.Sprintf("#ergo.services/ergo/net/proto/cachetest/VeryLongTypeName_%05d", i)
	}
	frames := cacheFrames(d)
	check.True(t, len(frames) > 1)
	c := cacheConn(edf.NewRegistry(), mock.NewLog())
	installFrames(t, c, frames)
	n := 0
	c.decodeOptions.RegCache.Range(func(_, _ any) bool { n++; return true })
	check.Equal(t, 2000, n)
}

func readCacheFrame(t *testing.T, r net.Conn) []byte {
	t.Helper()
	r.SetReadDeadline(time.Now().Add(2 * time.Second))
	hdr := make([]byte, 8)
	if _, err := io.ReadFull(r, hdr); err != nil {
		t.Fatalf("read frame header: %s", err)
	}
	body := make([]byte, int(binary.BigEndian.Uint32(hdr[2:6]))-8)
	if _, err := io.ReadFull(r, body); err != nil {
		t.Fatalf("read frame body: %s", err)
	}
	check.Equal(t, protoMessageC, hdr[7])
	return body
}

// the first message after a registration defines it on every link first, then
// goes by id.
func TestCacheDefineEveryLink(t *testing.T) {
	r := edf.NewRegistry()
	c := &connection{
		peer:        "peer@localhost",
		log:         mock.NewLog(),
		registry:    r,
		cacheUpdate: true,
	}
	c.snapshot = r.Revision()
	c.defined.Store(c.snapshot)
	c.encodeOptions = edf.Options{Registry: r, Revision: &c.defined, Cache: new(sync.Map)}

	var readers []net.Conn
	for i := 0; i < 2; i++ {
		srv, cli := net.Pipe()
		pi := &pool_item{fl: lib.NewFlusher(srv)}
		pi.connection.Store(&poolConn{srv})
		c.pool = append(c.pool, pi)
		readers = append(readers, cli)
		t.Cleanup(func() { pi.fl.Stop(); srv.Close(); cli.Close() })
	}

	check.NoError(t, r.RegisterType(cacheTestType{}))
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)
	done := make(chan error, 1)
	go func() { done <- c.encode(cacheTestType{A: 1}, buf) }()

	name := "#ergo.services/ergo/net/proto/cacheTestType"
	for _, rd := range readers {
		check.True(t, strings.Contains(string(readCacheFrame(t, rd)), name))
	}
	check.NoError(t, <-done)
	check.Equal(t, r.Revision(), c.defined.Load())
	check.False(t, strings.Contains(string(buf.B), name))
}

// a joining link starts with what the peer missed since the handshake.
func TestCachePrimeLink(t *testing.T) {
	r := edf.NewRegistry()
	c := &connection{log: mock.NewLog(), registry: r, snapshot: r.Revision()}
	check.NoError(t, r.RegisterAtom("cache.prime"))

	srv, cli := net.Pipe()
	pi := &pool_item{fl: lib.NewFlusher(srv)}
	pi.connection.Store(&poolConn{srv})
	t.Cleanup(func() { pi.fl.Stop(); srv.Close(); cli.Close() })

	go c.prime(pi)
	check.True(t, strings.Contains(string(readCacheFrame(t, cli)), "cache.prime"))
}

// NewConnection leaves the decode caches non-nil even when the peer advertised
// empty caches, so cache updates can be installed.
func TestNewConnectionDecodeCachesNonNil(t *testing.T) {
	result := gen.HandshakeResult{
		ConnectionID: "conn-1",
		Peer:         "peer@localhost",
		PeerCreation: 1,
		PoolSize:     1,
		Custom:       handshake.ConnectionOptions{PoolSize: 1, Registry: testRegistry}, // nil Decode* caches
	}
	conn, err := (&enp{}).NewConnection(mock.NewCore(), result, mock.NewLog())
	if err != nil {
		t.Fatal(err)
	}
	c := conn.(*connection)

	check.True(t, c.decodeOptions.AtomCache != nil)
	check.True(t, c.decodeOptions.RegCache != nil)
	check.True(t, c.decodeOptions.ErrCache != nil)
	installFrames(t, c, cacheFrames(edf.Dictionary{Atoms: map[uint16]gen.Atom{300: "worker"}}))
	atom, ok := c.decodeOptions.AtomCache.Load(uint16(300))
	check.True(t, ok)
	check.Equal(t, gen.Atom("worker"), atom)
}
