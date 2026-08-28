//go:build pprof

package node

import (
	"context"
	"errors"
	"runtime"
	"runtime/pprof"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

func (p *process) run() {
	if atomic.CompareAndSwapInt32(&p.state, int32(gen.ProcessStateSleep),
		int32(gen.ProcessStateRunning)) == false { // already running or terminated
		return
	}
	atomic.StoreInt64(&p.stateEntered, time.Now().UnixNano())
	atomic.AddUint64(&p.wakeups, 1)
	go func() {
		labels := pprof.Labels("pid", p.pid.String())
		pprof.Do(context.Background(), labels, func(context.Context) {
			if lib.Recover() {
				defer func() {
					if rcv := recover(); rcv != nil {
						pc, fn, line, _ := runtime.Caller(2)
						p.log.Panic("process terminated - %#v at %s[%s:%d]",
							rcv, runtime.FuncForPC(pc).Name(), fn, line)
						old := atomic.SwapInt32(&p.state, int32(gen.ProcessStateTerminated))
						if old == int32(gen.ProcessStateTerminated) {
							return
						}
						atomic.StoreInt64(&p.stateEntered, time.Now().UnixNano())
						reason := p.wrapPreserveMailbox(gen.TerminateReasonPanic)
						p.node.unregisterProcess(p, reason)
						p.node.finishProcess(p, reason, gen.TerminateReasonPanic)
					}
				}()
			}
		next:
			startTime := time.Now().UnixNano()
			// handle mailbox
			if err := p.behavior.ProcessRun(); err != nil {
				atomic.AddUint64(&p.runningTime, uint64(time.Now().UnixNano()-startTime))
				e := errors.Unwrap(err)
				if e == nil {
					e = err
				}
				if e != gen.TerminateReasonNormal && e != gen.TerminateReasonShutdown {
					p.log.Error("process terminated abnormally - %s", err)
				}

				old := atomic.SwapInt32(&p.state, int32(gen.ProcessStateTerminated))
				if old == int32(gen.ProcessStateTerminated) {
					return
				}
				atomic.StoreInt64(&p.stateEntered, time.Now().UnixNano())
				reason := p.wrapPreserveMailbox(e)
				p.node.unregisterProcess(p, reason)
				p.node.finishProcess(p, reason, err)
				return
			}

			// count the running time
			atomic.AddUint64(&p.runningTime, uint64(time.Now().UnixNano()-startTime))

			// change running state to sleep
			if atomic.CompareAndSwapInt32(
				&p.state,
				int32(gen.ProcessStateRunning),
				int32(gen.ProcessStateSleep),
			) == false {
				// process has been killed (was in zombee state)
				old := atomic.SwapInt32(&p.state, int32(gen.ProcessStateTerminated))
				if old == int32(gen.ProcessStateTerminated) {
					return
				}
				atomic.StoreInt64(&p.stateEntered, time.Now().UnixNano())
				reason := p.wrapPreserveMailbox(gen.TerminateReasonKill)
				p.node.unregisterProcess(p, reason)
				p.node.finishProcess(p, reason, gen.TerminateReasonKill)
				return
			}
			atomic.StoreInt64(&p.stateEntered, time.Now().UnixNano())
			// check if something left in the inbox and try to handle it
			if p.mailbox.Main.Item() == nil {
				if p.mailbox.System.Item() == nil {
					if p.mailbox.Urgent.Item() == nil {
						if p.mailbox.Log.Item() == nil {
							// inbox is emtpy
							return
						}
					}
				}
			}
			// we got a new messages. try to use this goroutine again
			if atomic.CompareAndSwapInt32(
				&p.state,
				int32(gen.ProcessStateSleep),
				int32(gen.ProcessStateRunning),
			) == false {
				// another goroutine is already running
				return
			}
			atomic.StoreInt64(&p.stateEntered, time.Now().UnixNano())
			goto next
		})
	}()
}
