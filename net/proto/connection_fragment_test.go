package proto

import (
	"encoding/binary"
	"testing"
	"time"

	"ergo.services/ergo/lib"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

// fragBuf builds one fragment frame as the assembly handlers see it: a 16-byte
// header (only bytes 8..16 are read: seq, index, total) followed by the payload.
func fragBuf(seqID uint32, idx, total uint16, payload string) *lib.Buffer {
	b := lib.TakeBuffer()
	b.Allocate(16)
	b.B[7] = protoMessageF
	binary.BigEndian.PutUint32(b.B[8:12], seqID)
	binary.BigEndian.PutUint16(b.B[12:14], idx)
	binary.BigEndian.PutUint16(b.B[14:16], total)
	b.B = append(b.B, payload...)
	return b
}

func TestHandleFragmentOrdered(t *testing.T) {
	c := &connection{log: mock.NewLog(), fragmentTimeout: time.Minute}
	asm := map[uint32]*fragmentAssembly{}

	check.Nil(t, c.handleFragmentOrdered(fragBuf(1, 0, 3, "foo"), asm))
	check.Nil(t, c.handleFragmentOrdered(fragBuf(1, 1, 3, "bar"), asm))

	r := c.handleFragmentOrdered(fragBuf(1, 2, 3, "baz"), asm)
	check.NotNil(t, r)
	check.Equal(t, "foobarbaz", string(r.B))
}

func TestHandleFragmentOrderedRejectsBadInput(t *testing.T) {
	c := &connection{log: mock.NewLog(), fragmentTimeout: time.Minute}
	asm := map[uint32]*fragmentAssembly{}

	short := lib.TakeBuffer()
	short.Allocate(8)
	check.Nil(t, c.handleFragmentOrdered(short, asm))                 // shorter than a header
	check.Nil(t, c.handleFragmentOrdered(fragBuf(2, 5, 3, "x"), asm)) // index out of range
}

func TestHandleFragmentUnorderedReassemblesByIndex(t *testing.T) {
	c := &connection{
		log:                   mock.NewLog(),
		fragmentTimeout:       time.Minute,
		maxFragmentAssemblies: 16,
		sharedFragments:       make(map[uint32]*fragmentAssembly),
		sharedFragTimer:       time.AfterFunc(time.Hour, func() {}),
	}
	c.sharedFragTimer.Stop()

	// arrive out of order; the result is ordered by fragment index
	check.Nil(t, c.handleFragmentUnordered(fragBuf(7, 2, 3, "baz")))
	check.Nil(t, c.handleFragmentUnordered(fragBuf(7, 0, 3, "foo")))

	r := c.handleFragmentUnordered(fragBuf(7, 1, 3, "bar"))
	check.NotNil(t, r)
	check.Equal(t, "foobarbaz", string(r.B))
}

func TestHandleFragmentUnorderedIgnoresDuplicate(t *testing.T) {
	c := &connection{
		log:                   mock.NewLog(),
		fragmentTimeout:       time.Minute,
		maxFragmentAssemblies: 16,
		sharedFragments:       make(map[uint32]*fragmentAssembly),
		sharedFragTimer:       time.AfterFunc(time.Hour, func() {}),
	}
	c.sharedFragTimer.Stop()

	check.Nil(t, c.handleFragmentUnordered(fragBuf(9, 0, 2, "foo")))
	check.Nil(t, c.handleFragmentUnordered(fragBuf(9, 0, 2, "dup"))) // duplicate index, ignored
	r := c.handleFragmentUnordered(fragBuf(9, 1, 2, "bar"))
	check.NotNil(t, r)
	check.Equal(t, "foobar", string(r.B))
}

func TestCleanupSharedFragments(t *testing.T) {
	c := &connection{
		log:             mock.NewLog(),
		sharedFragments: make(map[uint32]*fragmentAssembly),
		sharedFragTimer: time.AfterFunc(time.Hour, func() {}),
	}
	c.sharedFragTimer.Stop()
	c.sharedFragments[1] = &fragmentAssembly{deadline: time.Now().Add(-time.Minute)} // expired
	c.sharedFragments[2] = &fragmentAssembly{deadline: time.Now().Add(time.Hour)}    // fresh

	c.cleanupSharedFragments()

	_, hasExpired := c.sharedFragments[1]
	check.False(t, hasExpired)
	_, hasFresh := c.sharedFragments[2]
	check.True(t, hasFresh)
}

// newFragConn builds a connection ready for the unordered fragment path with the
// given concurrent-assembly cap.
func newFragConn(maxAssemblies int) *connection {
	c := &connection{
		log:                   mock.NewLog(),
		fragmentTimeout:       time.Minute,
		maxFragmentAssemblies: maxAssemblies,
		sharedFragments:       make(map[uint32]*fragmentAssembly),
		sharedFragTimer:       time.AfterFunc(time.Hour, func() {}),
	}
	c.sharedFragTimer.Stop()
	return c
}

// a fragment declaring more parts than maxFragmentCount is refused (ordered path).
func TestHandleFragmentOrderedRejectsTooManyFragments(t *testing.T) {
	c := &connection{log: mock.NewLog(), fragmentTimeout: time.Minute}
	asm := map[uint32]*fragmentAssembly{}
	check.Nil(t, c.handleFragmentOrdered(fragBuf(1, 0, maxFragmentCount+1, "x"), asm))
	check.Equal(t, 0, len(asm))
}

// same cap on the unordered path.
func TestHandleFragmentUnorderedRejectsTooManyFragments(t *testing.T) {
	c := newFragConn(16)
	check.Nil(t, c.handleFragmentUnordered(fragBuf(1, 0, maxFragmentCount+1, "x")))
	check.Equal(t, 0, len(c.sharedFragments))
}

// once the concurrent-assembly cap is reached, a fragment opening a new assembly
// is dropped while the existing one is kept.
func TestHandleFragmentUnorderedRejectsTooManyAssemblies(t *testing.T) {
	c := newFragConn(1)

	check.Nil(t, c.handleFragmentUnordered(fragBuf(1, 0, 2, "foo"))) // opens assembly seq=1 (incomplete)
	check.Nil(t, c.handleFragmentUnordered(fragBuf(2, 0, 2, "bar"))) // seq=2 exceeds the cap, dropped

	check.Equal(t, 1, len(c.sharedFragments))
	_, has1 := c.sharedFragments[1]
	check.True(t, has1)
	_, has2 := c.sharedFragments[2]
	check.False(t, has2)
}

// once the concurrent-assembly cap is reached on the ordered path, a fragment opening a
// new assembly is dropped while the existing one is kept.
func TestHandleFragmentOrderedRejectsTooManyAssemblies(t *testing.T) {
	c := &connection{log: mock.NewLog(), fragmentTimeout: time.Minute, maxFragmentAssemblies: 1}
	asm := map[uint32]*fragmentAssembly{}

	check.Nil(t, c.handleFragmentOrdered(fragBuf(1, 0, 2, "foo"), asm)) // opens assembly seq=1 (incomplete)
	check.Nil(t, c.handleFragmentOrdered(fragBuf(2, 0, 2, "bar"), asm)) // seq=2 exceeds the cap, dropped

	check.Equal(t, 1, len(asm))
	_, has1 := asm[1]
	check.True(t, has1)
	_, has2 := asm[2]
	check.False(t, has2)
}

// at the cap, a fragment opening a new assembly first evicts stale (expired) assemblies,
// so a dead sender streaming first-fragments cannot wedge the queue for a live one.
func TestHandleFragmentOrderedEvictsStaleOnCap(t *testing.T) {
	c := &connection{log: mock.NewLog(), fragmentTimeout: time.Minute, maxFragmentAssemblies: 1}
	asm := map[uint32]*fragmentAssembly{}
	asm[1] = &fragmentAssembly{ // stale assembly from a dead sender occupies the only slot
		totalFragments: 2,
		payloads:       make([][]byte, 0, 2),
		deadline:       time.Now().Add(-time.Minute),
	}

	check.Nil(t, c.handleFragmentOrdered(fragBuf(2, 0, 2, "foo"), asm)) // evicts seq=1, opens seq=2

	_, hasStale := asm[1]
	check.False(t, hasStale)
	_, hasNew := asm[2]
	check.True(t, hasNew)
	check.Equal(t, 1, len(asm))
	check.Equal(t, uint64(1), c.fragmentTimeouts.Load())
}

// a fragment whose total-count disagrees with the open assembly drops the assembly.
func TestHandleFragmentOrderedRejectsTotalMismatch(t *testing.T) {
	c := &connection{log: mock.NewLog(), fragmentTimeout: time.Minute}
	asm := map[uint32]*fragmentAssembly{}

	check.Nil(t, c.handleFragmentOrdered(fragBuf(1, 0, 3, "foo"), asm))
	check.Nil(t, c.handleFragmentOrdered(fragBuf(1, 1, 4, "bar"), asm)) // total 4 != 3

	_, exists := asm[1]
	check.False(t, exists)
}

// an assembly whose accumulated bytes exceed the receiver limit is rejected, and
// later fragments for that sequence are ignored.
func TestHandleFragmentUnorderedRejectsOversize(t *testing.T) {
	c := newFragConn(16)
	c.node_maxmessagesize = 4

	check.Nil(t, c.handleFragmentUnordered(fragBuf(1, 0, 2, "toolong"))) // 7 bytes > 4, rejected
	check.Nil(t, c.handleFragmentUnordered(fragBuf(1, 1, 2, "x")))       // assembly already rejected
}
