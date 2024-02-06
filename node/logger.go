package node

import (
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

//
// logger based on a process
//

func createProcessLogger(queue lib.QueueMPSC, run func()) gen.LoggerBehavior {
	return &processLogger{
		queue: queue,
		run:   run,
	}
}

type processLogger struct {
	queue lib.QueueMPSC
	level gen.LogLevel
	run   func()
}

func (p *processLogger) Log(message gen.MessageLog) {
	p.queue.Push(message)
	p.run()
}

func (p *processLogger) Terminate() {}
