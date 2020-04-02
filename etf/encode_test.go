package etf

import (
	"context"
	"github.com/halturin/ergo/lib"
	"reflect"
	"testing"
)

func TestEncodeBool(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	err := Encode(false, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, []byte{ettSmallAtom, 5, 'f', 'a', 'l', 's', 'e'}) {
		t.Fatal("incorrect value")
	}
}

func TestEncodeBoolWithAtomCache(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writerAtomCache := make(map[Atom]CacheItem)
	encodingAtomCache := TakeListAtomCache()
	defer ReleaseListAtomCache(encodingAtomCache)
	linkAtomCache := NewAtomCache(ctx)
	ci := CacheItem{ID: 499, Encoded: true, Name: "false"}

	writerAtomCache["false"] = ci

	err := Encode(false, b, linkAtomCache, writerAtomCache, encodingAtomCache)
	if err != nil {
		t.Fatal(err)
	}

	if encodingAtomCache.Len() != 1 || encodingAtomCache.L[0] != ci {
		t.Fatal("incorrect cache value")
	}

	if !reflect.DeepEqual(b.B, []byte{ettCacheRef, 0}) {
		t.Fatal("incorrect value")
	}

}

func BenchmarkEncodeBool(b *testing.B) {

	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Encode(false, buf, nil, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeBoolWithAtomCache(b *testing.B) {

	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writerAtomCache := make(map[Atom]CacheItem)
	encodingAtomCache := TakeListAtomCache()
	defer ReleaseListAtomCache(encodingAtomCache)
	linkAtomCache := NewAtomCache(ctx)

	writerAtomCache["false"] = CacheItem{ID: 499, Encoded: true, Name: "false"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Encode(false, buf, linkAtomCache, writerAtomCache, encodingAtomCache)
		encodingAtomCache.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}
}
