package local

import (
	"fmt"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

var (
	errIncorrect = fmt.Errorf("incorrect")
)

type initcase struct{}

type testcase struct {
	name   string
	input  any
	output any
	err    chan error
}

func (t *testcase) wait(timeout int) error {
	timer := time.NewTimer(time.Second * time.Duration(timeout))
	defer timer.Stop()
	select {
	case <-timer.C:
		return gen.ErrTimeout
	case e := <-t.err:
		return e
	}
}

type processTerminationWatcher struct {
	act.Actor

	watch watchProcess
}

type watchProcess struct {
	pid        gen.PID
	ready      chan<- struct{}
	terminated chan<- gen.PID
}

func (w *processTerminationWatcher) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case watchProcess:
		w.watch = m
		err := w.MonitorPID(m.pid)
		if err != nil {
			return fmt.Errorf("error monitoring PID %v: %v", m.pid, err)
		}
		close(w.watch.ready)
		return nil
	case gen.MessageDownPID:
		if m.PID == w.watch.pid {
			w.watch.terminated <- m.PID
			return nil
		}
	}
	return fmt.Errorf("unexpected message received: %v", message)
}

func factory_processTerminationWatcher() gen.ProcessBehavior {
	return &processTerminationWatcher{}
}

func (t *testcase) expectProcessToTerminate(pid gen.PID, process gen.Process, f func(p gen.Process) error) error {
	// start the watcher process
	watcherPid, err := process.Spawn(factory_processTerminationWatcher, gen.ProcessOptions{})
	if err != nil {
		return fmt.Errorf("error spawning watcher process")
	}

	// send a request to watch the pid that was provided
	ready := make(chan struct{})
	ch := make(chan gen.PID)
	process.Send(watcherPid, watchProcess{pid, ready, ch})

	// wait for the watcher process to be ready
	select {
	case <-ready:
	case <-time.After(1 * time.Second):
		return fmt.Errorf("timed out waiting for watcher")
	}

	// execute the function that we expect to trigger the process to terminate
	f(process)

	// wait at most one second for the watcher to observe the termination of the given process
	select {
	case msg := <-ch:
		if msg != pid {
			return fmt.Errorf("expected process %v to terminate but %v terminated instead", pid, msg)
		}
	case <-time.After(1 * time.Second):
		return fmt.Errorf("expected process %v to terminate", pid)
	}
	return nil
}
