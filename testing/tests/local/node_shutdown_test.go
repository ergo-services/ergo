package local

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type stubborn struct {
	act.Actor
	hold   time.Duration
	notify gen.PID
}

func factoryStubborn() gen.ProcessBehavior { return &stubborn{} }

func (a *stubborn) Init(args ...any) error {
	a.hold = args[0].(time.Duration)
	a.notify = args[1].(gen.PID)
	return nil
}

func (a *stubborn) HandleMessage(from gen.PID, message any) error {
	a.Send(a.notify, "started")
	time.Sleep(a.hold)
	return nil
}

func TestNodeShortInfo(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()
	n.Spawn(factoryT0, gen.ProcessOptions{})

	info, err := nd.ShortInfo()
	check.NoError(t, err)
	check.Equal(t, n.Name(), info.Name)
	check.Equal(t, int64(1), info.ProcessesTotal)
	check.Equal(t, uint64(1), info.ProcessesSpawned)
	check.True(t, info.Uptime >= 0)
	check.True(t, info.Framework.Name != "")
	check.Equal(t, 0, len(info.Peers))

	nd.Stop()
	nd.Wait()

	_, err = nd.ShortInfo()
	check.ErrorIs(t, err, gen.ErrNodeTerminated)
}

func TestNodeWaitReturnsWhenTheNodeStops(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()

	returned := make(chan struct{})
	go func() {
		nd.Wait()
		close(returned)
	}()

	select {
	case <-returned:
		t.Fatal("Wait returned while the node was still running")
	case <-time.After(200 * time.Millisecond):
	}

	nd.StopWithTimeout(0)

	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after the node stopped")
	}

	check.True(t, nd.IsAlive() == false)
	nd.Wait()
	check.ErrorIs(t, nd.WaitWithTimeout(time.Second), gen.ErrNodeTerminated)
}

func TestNodeShutdownForceKillsABusyProcess(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()

	sink := n.Spawn(factoryTarget, gen.ProcessOptions{})
	pid := n.Spawn(factoryStubborn, gen.ProcessOptions{}, 2*time.Second, sink)
	n.Send(pid, "busy")
	n.ShouldSend().From(pid).To(sink).Message("started").Once().Within(5 * time.Second).Must()

	stopped := make(chan struct{})
	go func() {
		nd.StopWithTimeout(100 * time.Millisecond)
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(15 * time.Second):
		t.Fatal("the node did not come back from a shutdown with a busy process")
	}

	check.True(t, nd.IsAlive() == false)
}

func TestNodeStopIsIdempotent(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()

	nd.Stop()
	nd.Stop()
	nd.StopForce()
	nd.StopWithTimeout(time.Second)

	check.True(t, nd.IsAlive() == false)
}
