package local

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/node"
)

var (
	t14cases []*testcase
)

func factory_t14() gen.ProcessBehavior {
	return &t14{}
}

type t14 struct {
	act.Actor

	testcase *testcase
}

func (t *t14) HandleMessage(from gen.PID, message any) error {
	if t.testcase == nil {
		t.testcase = message.(*testcase)
		message = initcase{}
	}
	// get method by name
	method := reflect.ValueOf(t).MethodByName(t.testcase.name)
	if method.IsValid() == false {
		t.testcase.err <- fmt.Errorf("unknown method %q", t.testcase.name)
		t.testcase = nil
		return nil
	}
	method.Call([]reflect.Value{reflect.ValueOf(message)})
	return nil
}

func factory_t14pool() gen.ProcessBehavior {
	return &t14pool{}
}

type t14pool struct {
	act.Pool
	tc *testcase
}

func (p *t14pool) Init(args ...any) (act.PoolOptions, error) {
	var options act.PoolOptions
	options.WorkerFactory = factory_t14worker
	options.PoolSize = 5
	return options, nil
}

func (p *t14pool) HandleMessage(from gen.PID, message any) error {
	p.Log().Info("pool process got message from %s: %s", from, message)
	select {
	case p.tc.err <- nil:
	default:
	}
	return nil
}

func (p *t14pool) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	p.Log().Info("pool process got request from %s", from)
	switch v := request.(type) {
	case *testcase:
		p.tc = v
	case int:
		if v > 0 {
			p.Log().Info("start %d new workers", v)
			n, err := p.AddWorkers(v)
			if err != nil {
				p.Log().Error("unable to add %d workers:%s", v, err)
				return 0, nil
			}
			p.Log().Info("total num of workers now: %d", n)
			return n, nil
		}

		p.Log().Info("stop %d workers", v)
		n, err := p.RemoveWorkers(-v)
		if err != nil {
			p.Log().Error("unable to add %d workers:%s", v, err)
			return 0, nil
		}
		p.Log().Info("total num of workers now: %d", n)
		return n, nil
	}
	return true, nil
}

func (p *t14pool) Terminate(reason error) {
	p.Log().Info("pool process terminated: %s", reason)
}

func factory_t14worker() gen.ProcessBehavior {
	return &t14worker{}
}

type t14worker struct {
	act.Actor
	id int32
	tc *testcase
}

var poolworkerid int32

func (w *t14worker) Init(args ...any) error {
	w.Log().Info("started worker")
	w.id = atomic.AddInt32(&poolworkerid, 1)
	return nil
}

func (w *t14worker) HandleMessage(from gen.PID, message any) error {
	w.Log().Info("worker process got message from %s", from)
	w.tc.output = w.id
	select {
	case w.tc.err <- nil:
	default:
	}
	return nil
}

func (w *t14worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	w.Log().Info("worker process got request from %s", from)
	w.tc = request.(*testcase)
	return w.id, nil
}

func (p *t14worker) Terminate(reason error) {
	p.Log().Info("worker process id=%d terminated: %s", p.id, reason)
}

func (t *t14) TestBasic(input any) {
	defer func() {
		t.testcase = nil
	}()

	poolpid, err := t.Spawn(factory_t14pool, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn pool process: %s", err)
		t.testcase.err <- err
		return
	}

	pris := []gen.MessagePriority{gen.MessagePriorityHigh, gen.MessagePriorityMax}
	tc := &testcase{"", nil, nil, make(chan error)}
	// test call request to the pool process. must be handled by itself due to high priority
	for _, p := range pris {
		t.Log().Info("making a call with priority %s to the pool process", p)
		v, err := t.CallWithPriority(poolpid, tc, p)
		if err != nil {
			t.Log().Error("call pool process failed: %s", err)
			t.testcase.err <- err
			return
		}
		if res, ok := v.(bool); ok == false || res != true {
			t.Log().Error("incorrect result: %v (exp: true)", res)
			t.testcase.err <- err
			return
		}
	}

	// test sending a message to the pool process. must be handled by itself due to high priority
	for _, p := range pris {
		t.Log().Info("send with priority %s to the pool process", p)
		err := t.SendWithPriority(poolpid, "hi", p)
		if err != nil {
			t.Log().Error("sending to the pool process failed: %s", err)
			t.testcase.err <- err
			return
		}
		if err := tc.wait(1); err != nil {
			t.Log().Error("got error", err)
			t.testcase.err <- err
			return
		}
	}

	// test call forwarding to the worker processes
	for i := int32(0); i < 10; i++ {
		expid := i%5 + 1
		t.Log().Info("making a call to the pool process. must be forwared. i=%d , exp=%d", i, expid)
		v, err := t.Call(poolpid, tc)
		if err != nil {
			t.Log().Error("call worker process failed: %s", err)
			t.testcase.err <- err
			return
		}
		// check worker id
		if id, ok := v.(int32); ok == false || id != expid {
			t.Log().Error("incorrect worker id: %d (exp: %d)", id, expid)
			t.testcase.err <- errIncorrect
			return
		}
	}

	// test send forwarding to the worker processes
	for i := int32(0); i < 10; i++ {
		expid := i%5 + 1
		t.Log().Info("sending a message to the pool process. must be forwared. i=%d , exp=%d", i, expid)
		err := t.Send(poolpid, tc)
		if err != nil {
			t.Log().Error("call worker process failed: %s", err)
			t.testcase.err <- err
			return
		}
		if err := tc.wait(1); err != nil {
			t.Log().Error("got error", err)
			t.testcase.err <- err
			return
		}
		// check worker id
		if id, ok := tc.output.(int32); ok == false || id != expid {
			t.Log().Error("incorrect worker id: %d (exp: %d)", id, expid)
			t.testcase.err <- errIncorrect
			return
		}
	}

	// ask pool process to add 3 workers
	v, err := t.CallWithPriority(poolpid, 3, gen.MessagePriorityHigh)
	if err != nil {
		t.Log().Error("call pool process failed: %s", err)
		t.testcase.err <- err
		return
	}
	if res, ok := v.(int64); ok == false || res != 8 {
		t.Log().Error("incorrect result: %v (exp: 8)", v)
		t.testcase.err <- errIncorrect
		return
	}

	// test call forwarding with updated pool of workers
	for i := int32(0); i < 16; i++ {
		expid := i%8 + 1
		t.Log().Info("making a call to the pool process. must be forwared. i=%d , exp=%d", i, expid)
		v, err := t.Call(poolpid, tc)
		if err != nil {
			t.Log().Error("call worker process failed: %s", err)
			t.testcase.err <- err
			return
		}
		// check worker id
		if id, ok := v.(int32); ok == false || id != expid {
			t.Log().Error("incorrect worker id: %d (exp: %d)", id, expid)
			t.testcase.err <- errIncorrect
			return
		}
	}

	// ask pool process to remove 5 workers
	v, err = t.CallWithPriority(poolpid, -5, gen.MessagePriorityHigh)
	if err != nil {
		t.Log().Error("call pool process failed: %s", err)
		t.testcase.err <- err
		return
	}
	if res, ok := v.(int64); ok == false || res != 3 {
		t.Log().Error("incorrect result: %v (exp: 3)", v)
		t.testcase.err <- errIncorrect
		return
	}

	// test call forwarding with updated pool of workers
	for i := int32(0); i < 10; i++ {
		expid := i%3 + 6
		t.Log().Info("making a call to the pool process. must be forwared. i=%d , exp=%d", i, expid)
		v, err := t.Call(poolpid, tc)
		if err != nil {
			t.Log().Error("call worker process failed: %s", err)
			t.testcase.err <- err
			return
		}
		// check worker id
		if id, ok := v.(int32); ok == false || id != expid {
			t.Log().Error("incorrect worker id: %d (exp: %d)", id, expid)
			t.testcase.err <- errIncorrect
			return
		}
	}
	t.testcase.err <- nil
}

func TestT14Pool(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	//nopt.Log.Level = gen.LogLevelTrace
	node, err := node.Start("t14node@localhost", nopt, gen.Version{})
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t14, popt)
	if err != nil {
		panic(err)
	}

	t14cases = []*testcase{
		{"TestBasic", nil, nil, make(chan error)},
	}
	for _, tc := range t14cases {
		name := tc.name
		if tc.input != nil {
			name = fmt.Sprintf("%s:%s", tc.name, tc.input)
		}
		t.Run(name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(30); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
