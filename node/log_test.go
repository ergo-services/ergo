package node

import (
	"sync"
	"testing"

	"ergo.services/ergo/gen"
)

func TestLogLevelConcurrent(t *testing.T) {
	l := createLog(gen.LogLevelInfo, func(gen.MessageLog, string) {})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			l.SetLevel(gen.LogLevelDebug)
			l.SetLevel(gen.LogLevelError)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			l.Info("msg %d", i) // write() reads l.level
			_ = l.Level()       // spawn-inheritance path reads l.level
		}
	}()
	wg.Wait()
}
