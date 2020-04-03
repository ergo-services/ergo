package etf

import (
	"context"
	"fmt"
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

type integerCase struct {
	name     string
	integer  interface{}
	expected []byte
}

func integerCases() []integerCase {
	return []integerCase{
		integerCase{"uint8: 255", uint8(255), []byte{ettSmallInteger, 255}},
		integerCase{"uint16: 255", uint16(255), []byte{ettSmallInteger, 255}},
		integerCase{"uint32: 255", uint32(255), []byte{ettSmallInteger, 255}},
		integerCase{"uint64: 255", uint64(255), []byte{ettSmallInteger, 255}},
		integerCase{"uint: 255", uint(255), []byte{ettSmallInteger, 255}},

		integerCase{"uint16: 256", uint16(256), []byte{ettInteger, 0, 0, 1, 0}},

		integerCase{"uint16: 65535", uint16(65535), []byte{ettInteger, 0, 0, 255, 255}},
		integerCase{"uint32: 65535", uint32(65535), []byte{ettInteger, 0, 0, 255, 255}},
		integerCase{"uint64: 65535", uint64(65535), []byte{ettInteger, 0, 0, 255, 255}},

		integerCase{"uint64: 65536", uint64(65536), []byte{ettInteger, 0, 1, 0, 0}},

		integerCase{"uint32: 4294967295", uint32(4294967295), []byte{ettSmallBig, 4, 0, 255, 255, 255, 255}},
		integerCase{"uint64: 4294967295", uint64(4294967295), []byte{ettSmallBig, 4, 0, 255, 255, 255, 255}},

		integerCase{"uint64: 4294967296", uint64(4294967296), []byte{ettSmallBig, 5, 0, 0, 0, 0, 0, 1}},

		integerCase{"int8: -127", int8(-127), []byte{ettInteger, 255, 255, 255, 129}},
	}
}

func TestEncodeInteger(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	for _, c := range integerCases() {
		t.Run(c.name, func(t *testing.T) {
			b.Reset()

			err := Encode(c.integer, b, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(b.B, c.expected) {
				fmt.Println("exp ", c.expected)
				fmt.Println("got ", b.B)
				t.Fatal("incorrect value")
			}
		})
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

func BenchmarkEncodeInteger(b *testing.B) {
	for _, c := range integerCases() {
		b.Run(c.name, func(b *testing.B) {
			buf := lib.TakeBuffer()
			defer lib.ReleaseBuffer(buf)

			for i := 0; i < b.N; i++ {
				err := Encode(c.integer, buf, nil, nil, nil)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
