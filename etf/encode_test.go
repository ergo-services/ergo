package etf

import (
	"github.com/halturin/ergo/lib"
	"testing"
)

func TestEncodingDistHeaderAtomCache(t *testing.T) {

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	writerAtomCache := make(map[Atom]CacheItem)
	encodingAtomCache := make([]CacheItem, 0, 100)

	l := &Link{}
	l.encodeDistHeaderAtomCache(b, writerAtomCache, encodingAtomCache)

}
