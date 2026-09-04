package proto

import (
	"testing"

	"ergo.services/ergo/testing/check"
)

func TestMedianInt64(t *testing.T) {
	check.Equal(t, int64(0), medianInt64(nil))
	check.Equal(t, int64(20), medianInt64([]int64{30, 10, 20})) // odd count
	check.Equal(t, int64(15), medianInt64([]int64{10, 20}))     // even count -> average of the middle pair
}

func TestPushSkew(t *testing.T) {
	c := &connection{}
	pi := &pool_item{}

	for _, s := range []int64{10, 30, 20} {
		c.pushSkew(pi, s)
	}

	check.Equal(t, int32(3), pi.skewCount.Load())
	check.Equal(t, int64(20), pi.skewValue.Load()) // median of {10,30,20}
}

func TestPushSkewCapsAtRingSize(t *testing.T) {
	c := &connection{}
	pi := &pool_item{}

	for i := 0; i < skewRingSize+5; i++ {
		c.pushSkew(pi, int64(i))
	}

	check.Equal(t, int32(skewRingSize), pi.skewCount.Load())
}

func TestSkewMedianAcrossPool(t *testing.T) {
	c := &connection{}
	for _, v := range []int64{100, 300, 200} {
		pi := &pool_item{}
		pi.skewValue.Store(v)
		pi.skewCount.Store(1)
		c.pool = append(c.pool, pi)
	}

	check.Equal(t, int64(200), c.Skew()) // median across the pool
}

func TestSkewIgnoresUnmeasuredPoolItems(t *testing.T) {
	c := &connection{}
	measured := &pool_item{}
	measured.skewValue.Store(50)
	measured.skewCount.Store(1)
	c.pool = append(c.pool, measured, &pool_item{}) // second item has no measurement

	check.Equal(t, int64(50), c.Skew())
}

func TestSkewEmptyPool(t *testing.T) {
	c := &connection{}
	check.Equal(t, int64(0), c.Skew())
}
