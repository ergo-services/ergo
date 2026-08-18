package node

import (
	"sync/atomic"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// the whole counter must be carried: ID[0] the low 18 bits, ID[1] the rest.
func TestMakeRefCarriesWholeCounter(t *testing.T) {
	n := &node{name: "test@localhost"}

	ref := n.MakeRef()
	id := atomic.LoadUint64(&n.uniqID)
	check.Equal(t, id&((1<<18)-1), ref.ID[0])
	check.Equal(t, id>>18, ref.ID[1])
}

// refs must stay unique across the window that used to wrap at 2^18.
func TestMakeRefNoWrapWithinWindow(t *testing.T) {
	n := &node{name: "test@localhost"}

	const window = 1 << 18
	seen := make(map[gen.Ref]struct{}, window+2)
	for i := 0; i < window+2; i++ {
		ref := n.MakeRef()
		if _, duplicate := seen[ref]; duplicate {
			t.Fatalf("duplicate ref %s after %d refs", ref, i+1)
		}
		seen[ref] = struct{}{}
	}
}
