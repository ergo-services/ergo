package node

import (
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

//
// logger based on a process
//

func createProcessLogger(queue lib.QueueMPSC, run func()) gen.LoggerBehavior {
	return &process_logger{
		queue: queue,
		run:   run,
	}
}

type process_logger struct {
	queue lib.QueueMPSC
	level gen.LogLevel
	run   func()
}

func (p *process_logger) Log(message gen.MessageLog) {
	p.queue.Push(message)
	p.run()
}

func (p *process_logger) Terminate() {}
