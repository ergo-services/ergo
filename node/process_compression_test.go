package node

import (
	"sync"
	"testing"

	"ergo.services/ergo/gen"
)

func TestProcessCompressionConcurrent(t *testing.T) {
	p := &process{}
	p.compression.Store(&gen.Compression{
		Type:      gen.DefaultCompressionType,
		Level:     gen.DefaultCompressionLevel,
		Threshold: gen.DefaultCompressionThreshold,
	})

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			p.updateCompression(func(c *gen.Compression) { c.Enable = i%2 == 0 })
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			p.updateCompression(func(c *gen.Compression) { c.Threshold = gen.DefaultCompressionThreshold + i })
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			_ = p.Compression()
			_ = *p.compression.Load()
		}
	}()
	wg.Wait()
}
