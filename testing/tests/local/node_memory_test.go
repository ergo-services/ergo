package local

import (
	"runtime"
	"runtime/debug"
	"testing"

	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// TestLocalNodeMemoryUsedPeak: MemoryUsed is what the runtime holds now and falls once
// the heap goes back to the OS, while MemoryPeak keeps the high-water mark. Both Info
// and ShortInfo report them.
func TestLocalNodeMemoryUsedPeak(t *testing.T) {
	const size = 256 << 20

	s := stage.New(t)
	nd := s.StartNode("mem", stage.NodeOptions{}).Native()

	block := make([]byte, size)
	for i := 0; i < len(block); i += 4096 {
		block[i] = 1
	}

	held, err := nd.Info()
	check.NoError(t, err)
	check.True(t, held.MemoryUsed >= size)
	check.True(t, held.MemoryPeak >= held.MemoryUsed)
	runtime.KeepAlive(block)

	debug.FreeOSMemory()

	freed, err := nd.Info()
	check.NoError(t, err)
	check.True(t, freed.MemoryUsed+size/2 < held.MemoryUsed)
	check.True(t, freed.MemoryPeak >= held.MemoryPeak)

	short, err := nd.ShortInfo()
	check.NoError(t, err)
	check.True(t, short.MemoryUsed+size/2 < held.MemoryUsed)
	check.True(t, short.MemoryPeak >= held.MemoryPeak)
}
