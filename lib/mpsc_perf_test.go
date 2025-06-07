package lib

import (
	"testing"
	"time"
)

// Benchmark MPSC queue performance (using optimized implementation)
func BenchmarkMPSCPerformance(b *testing.B) {
	b.Run("SingleProducer_MixedOps", benchmarkSingleProducerMixed)
	b.Run("PushOnly", benchmarkPushOnly)
	b.Run("SingleOperation", benchmarkSingleOperation)
	b.Run("MemoryAllocation", benchmarkMemoryAllocation)
}

func benchmarkSingleProducerMixed(b *testing.B) {
	queue := NewQueueMPSC()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		counter := 0
		for pb.Next() {
			if counter%2 == 0 {
				queue.Push(counter)
			} else {
				queue.Pop()
			}
			counter++
		}
	})
}

func benchmarkPushOnly(b *testing.B) {
	queue := NewQueueMPSC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		queue.Push(i)
	}
}

func benchmarkSingleOperation(b *testing.B) {
	queue := NewQueueMPSC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		start := time.Now()
		queue.Push(i)
		queue.Pop()
		_ = time.Since(start)
	}
}

func benchmarkMemoryAllocation(b *testing.B) {
	queue := NewQueueMPSC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		queue.Push(i)
		queue.Pop()
	}
}
