package tm

import (
	"sync/atomic"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// states for dispatcher
const (
	tmStateSleep int32 = iota
	tmStateRunning
)

type dispatcher struct {
	tm    *targetManager
	core  gen.CoreTargetManager
	queue lib.QueueMPSC
	state int32
}

type dispatchTask struct {
	from    gen.PID
	to      gen.PID
	options gen.MessageOptions
	message any
}

type dispatchBatch struct {
	tasks []*dispatchTask
}

type dispatchRemoteEvent struct {
	node    gen.Atom
	from    gen.PID
	options gen.MessageOptions
	message gen.MessageEvent
}

func newDispatcher(tm *targetManager, core gen.CoreTargetManager) *dispatcher {
	return &dispatcher{
		tm:    tm,
		core:  core,
		queue: lib.NewQueueMPSC(),
		state: tmStateSleep,
	}
}

func (d *dispatcher) push(task *dispatchTask) {
	d.queue.Push(task)
	d.wakeup()
}

func (d *dispatcher) pushBatch(batch *dispatchBatch) {
	if batch == nil || len(batch.tasks) == 0 {
		return
	}
	d.queue.Push(batch)
	d.wakeup()
}

func (d *dispatcher) pushRemoteEvent(task *dispatchRemoteEvent) {
	d.queue.Push(task)
	d.wakeup()
}

func (d *dispatcher) wakeup() {
	if atomic.CompareAndSwapInt32(&d.state, tmStateSleep, tmStateRunning) {
		go d.run()
	}
}

func (d *dispatcher) run() {
next:
	for {
		item, ok := d.queue.Pop()
		if ok == false {
			break
		}

		switch v := item.(type) {
		case *dispatchTask:
			d.deliver(v)
		case *dispatchBatch:
			for _, task := range v.tasks {
				d.deliver(task)
			}
		case *dispatchRemoteEvent:
			d.deliverRemoteEvent(v)
		}
	}

	if atomic.CompareAndSwapInt32(&d.state, tmStateRunning, tmStateSleep) == false {
		return
	}

	if d.queue.Item() == nil {
		return
	}

	if atomic.CompareAndSwapInt32(&d.state, tmStateSleep, tmStateRunning) {
		goto next
	}
}

func (d *dispatcher) deliver(task *dispatchTask) {
	// Check message type
	switch task.message.(type) {
	case gen.MessageExitPID, gen.MessageExitProcessID, gen.MessageExitAlias, gen.MessageExitEvent, gen.MessageExitNode:
		// Exit messages - send with preserved type
		d.core.RouteSendExitMessage(task.from, task.to, task.message)

		if d.tm != nil {
			d.tm.exitSignalsDelivered.Add(1)
		}

	case gen.MessageDownPID, gen.MessageDownProcessID, gen.MessageDownAlias, gen.MessageDownEvent, gen.MessageDownNode:
		// Down messages
		d.core.RouteSendPID(task.from, task.to, task.options, task.message)

		if d.tm != nil {
			d.tm.downMessagesDelivered.Add(1)
		}

	case gen.MessageEvent:
		// Event delivery - use RouteSendEventMessage for proper MailboxMessageTypeEvent
		d.core.RouteSendEventMessage(task.from, task.to, task.options, task.message.(gen.MessageEvent))

		if d.tm != nil {
			d.tm.eventsSent.Add(1)
		}

	default:
		// Other messages (EventStart, EventStop)
		d.core.RouteSendPID(task.from, task.to, task.options, task.message)
	}
}

func (d *dispatcher) deliverRemoteEvent(task *dispatchRemoteEvent) {
	connection, err := d.core.GetConnection(task.node)
	if err != nil {
		return
	}
	connection.SendEvent(task.from, task.options, task.message)

	if d.tm != nil {
		d.tm.eventsSent.Add(1)
	}
}
