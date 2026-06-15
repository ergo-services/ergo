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
