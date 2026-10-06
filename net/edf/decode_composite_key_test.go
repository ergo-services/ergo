package edf

import (
	"reflect"
	"sync"
	"testing"

	"ergo.services/ergo/lib"
)

type testArrayKeys struct {
	Arrays map[[2]int]string
	Nested map[[2][2]int8][]string
}

func TestDecodeCompositeMapKeys(t *testing.T) {
	if err := RegisterTypeOf(testArrayKeys{}); err != nil {
		t.Fatalf("register: %s", err)
	}
	key := 7
	values := []any{
		testArrayKeys{
			Arrays: map[[2]int]string{{1, 2}: "a", {3, 4}: "b"},
			Nested: map[[2][2]int8][]string{{{1, 2}, {3, 4}}: {"x"}},
		},
		map[[2]int]string{{5, 6}: "c"},
		map[[2]int][2]int{{1, 1}: {2, 2}},
		map[*int]string{&key: "p"},
	}
	options := Options{Cache: new(sync.Map)}
	for _, v := range values {
		for round := 0; round < 2; round++ {
			b := lib.TakeBuffer()
			if err := Encode(v, b, options); err != nil {
				t.Fatalf("encode %T: %s", v, err)
			}
			got, rest, err := Decode(b.B, options)
			if err != nil {
				t.Fatalf("decode %T (round %d): %s", v, round, err)
			}
			if len(rest) > 0 {
				t.Fatalf("decode %T: %d bytes left", v, len(rest))
			}
			if _, isPtrMap := v.(map[*int]string); isPtrMap {
				for k, s := range got.(map[*int]string) {
					if *k != key || s != "p" {
						t.Errorf("decoded %v: %s", *k, s)
					}
				}
			} else if reflect.DeepEqual(got, v) == false {
				t.Errorf("decoded %#v, want %#v", got, v)
			}
			lib.ReleaseBuffer(b)
		}
	}
}
