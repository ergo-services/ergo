package act_test

import (
	"errors"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

// test pool: only Init is custom; HandleMessage records admin (high-priority) traffic.
type plu struct {
	act.Pool
	hits []string
}

func factoryPlu() gen.ProcessBehavior { return &plu{} }

func (p *plu) Init(args ...any) (act.PoolOptions, error) {
	return args[0].(act.PoolOptions), nil
}
func (p *plu) HandleMessage(from gen.PID, message any) error {
	if message == "die" {
		return errActorBoom
	}
	p.hits = append(p.hits, "admin")
	return nil
}
func (p *plu) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request {
	case "ping":
		return "pong", nil
	case "fail":
		return nil, errActorBoom
	case "normal":
		return "bye", gen.TerminateReasonNormal
	}
	return nil, nil // async
}

func plb(s *unit.Subject) *plu { return s.Behavior().(*plu) }

func poolOpts(size int64) act.PoolOptions {
	return act.PoolOptions{PoolSize: size, WorkerFactory: factoryRouteWorker}
}

// spawnPool spawns the pool and returns it plus the worker PIDs in pool order.
func spawnPool(t *testing.T, size int64) (*unit.Subject, []gen.PID) {
	t.Helper()
	s, err := unit.Spawn(t, factoryPlu, gen.ProcessOptions{}, poolOpts(size))
	check.NoError(t, err)
	spawns := s.ShouldSpawn().Collect()
	pids := make([]gen.PID, len(spawns))
	for i, sp := range spawns {
		pids[i] = sp.Child
	}
	return s, pids
}

//
// init
//

// Init spawns PoolSize workers (anonymously, linked to the parent).
func TestPoolUnitInitSpawnsWorkers(t *testing.T) {
	s, pids := spawnPool(t, 3)
	check.Equal(t, 3, len(pids))
	for _, sp := range s.ShouldSpawn().Collect() {
		check.Equal(t, gen.Atom(""), sp.Register)
		check.True(t, sp.Options.LinkParent)
	}
}

// PoolSize < 1 falls back to the default pool size.
func TestPoolUnitInitDefaultSize(t *testing.T) {
	s, _ := unit.Spawn(t, factoryPlu, gen.ProcessOptions{}, poolOpts(0))
	s.ShouldSpawn().AtLeast(1).Assert() // default size workers spawned
}

func TestPoolUnitInitPanic(t *testing.T) {
	_, err := unit.Spawn(t, factoryPluPanic, gen.ProcessOptions{}, poolOpts(1))
	check.Error(t, err)
}

//
// routing
//

// normal-priority sends are forwarded to the workers round-robin.
func TestPoolUnitForwardRoundRobin(t *testing.T) {
	s, pids := spawnPool(t, 3)
	mark := s.Mark()
	s.SendMessage(gen.PID{}, "m1")
	s.SendMessage(gen.PID{}, "m2")
	s.SendMessage(gen.PID{}, "m3")

	fwd := s.ShouldForward().Since(mark).Collect()
	check.Equal(t, 3, len(fwd))
	check.Equal(t, pids[0], fwd[0].To)
	check.Equal(t, pids[1], fwd[1].To)
	check.Equal(t, pids[2], fwd[2].To)
}

// a dead worker is respawned on the next forward.
func TestPoolUnitForwardRespawn(t *testing.T) {
	s, pids := spawnPool(t, 1)
	s.OnForward(pids[0]).Fail(gen.ErrProcessTerminated)
	mark := s.Mark()
	s.SendMessage(gen.PID{}, "m")
	s.ShouldSpawn().Since(mark).Once().Assert() // worker respawned
}

// high-priority sends reach the admin HandleMessage (not forwarded).
func TestPoolUnitAdminHandleMessage(t *testing.T) {
	s, _ := spawnPool(t, 2)
	mark := s.Mark()
	s.SendMessageWithPriority(gen.PID{}, "hi", gen.MessagePriorityHigh)
	s.ShouldForward().Since(mark).None().Assert()
	check.Equal(t, []string{"admin"}, plb(s).hits)
}

func TestPoolUnitAdminHandleMessageError(t *testing.T) {
	s, _ := spawnPool(t, 1)
	s.SendMessageWithPriority(gen.PID{}, "die", gen.MessagePriorityHigh)
	check.True(t, s.Terminated())
}

//
// worker management
//

func TestPoolUnitAddWorkers(t *testing.T) {
	s, _ := spawnPool(t, 2)
	mark := s.Mark()
	n, err := plb(s).AddWorkers(2)
	check.NoError(t, err)
	check.Equal(t, int64(4), n)
	s.ShouldSpawn().Since(mark).Times(2).Assert()
}

func TestPoolUnitRemoveWorkers(t *testing.T) {
	s, pids := spawnPool(t, 3)
	mark := s.Mark()
	n, err := plb(s).RemoveWorkers(1)
	check.NoError(t, err)
	check.Equal(t, int64(2), n)
	s.ShouldSendExit().To(pids[0]).Since(mark).Once().Assert()

	_, err = plb(s).RemoveWorkers(100) // drains then errors
	check.ErrorIs(t, err, act.ErrPoolEmpty)
}

// a worker exit signal terminates the pool.
func TestPoolUnitWorkerExitTerminates(t *testing.T) {
	s, pids := spawnPool(t, 2)
	s.DeliverExit(pids[0], errors.New("crash"))
	check.True(t, s.Terminated())
}

// every non-PID exit variant terminates the pool too.
func TestPoolUnitExitVariantsTerminate(t *testing.T) {
	for _, ev := range exitVariants() {
		s, _ := spawnPool(t, 1)
		s.DeliverExitMessage(ev)
		check.True(t, s.Terminated())
	}
}

// a worker with a full mailbox is skipped and the next worker takes the message.
func TestPoolUnitForwardMailboxFullNextWorker(t *testing.T) {
	s, pids := spawnPool(t, 2)
	s.OnForward(pids[0]).Fail(gen.ErrProcessMailboxFull) // first worker is full
	mark := s.Mark()
	s.SendMessage(gen.PID{}, "m")
	s.ShouldForward().To(pids[1]).Since(mark).Once().Assert() // delivered to the second
}

// when every worker is full the message is counted as unhandled.
func TestPoolUnitForwardAllFullUnhandled(t *testing.T) {
	s, pids := spawnPool(t, 1)
	s.OnForward(pids[0]).Fail(gen.ErrProcessMailboxFull)
	s.SendMessage(gen.PID{}, "m")
	m, err := s.Inspect(gen.PID{})
	check.NoError(t, err)
	check.Equal(t, "1", m["messages_unhandled"])
}

// a high-priority Call reaches the admin HandleCall (not forwarded to a worker).
func TestPoolUnitAdminHandleCall(t *testing.T) {
	s, _ := spawnPool(t, 2)
	mark := s.Mark()
	resp, err := s.CallWithPriority(gen.PID{}, "ping", gen.MessagePriorityHigh)
	check.NoError(t, err)
	check.Equal(t, "pong", resp)
	s.ShouldForward().Since(mark).None().Assert()
}

func TestPoolUnitAdminHandleCallError(t *testing.T) {
	s, _ := spawnPool(t, 1)
	_, err := s.CallWithPriority(gen.PID{}, "fail", gen.MessagePriorityHigh)
	check.ErrorIs(t, err, errActorBoom)
	check.True(t, s.Terminated())
}

func TestPoolUnitAdminHandleCallNormalResult(t *testing.T) {
	s, _ := spawnPool(t, 1)
	resp, err := s.CallWithPriority(gen.PID{}, "normal", gen.MessagePriorityHigh)
	check.NoError(t, err)
	check.Equal(t, "bye", resp)
	check.True(t, s.Terminated())
}

func TestPoolUnitAdminHandleCallAsync(t *testing.T) {
	s, _ := spawnPool(t, 1)
	resp, err := s.CallWithPriority(gen.PID{}, "q", gen.MessagePriorityHigh) // nil -> async
	check.NoError(t, err)
	check.Nil(t, resp)
}

//
// inspect / kind / default callbacks
//

func TestPoolUnitInspect(t *testing.T) {
	s, _ := spawnPool(t, 2)
	m, err := s.Inspect(gen.PID{})
	check.NoError(t, err)
	check.NotNil(t, m)
}

func TestPoolUnitKind(t *testing.T) {
	s, _ := spawnPool(t, 1)
	kind := s.Behavior().(interface{ ProcessKind() gen.ProcessKind }).ProcessKind()
	check.Equal(t, gen.ProcessKindPool, kind)
}

type pluPlain struct{ act.Pool }

func factoryPluPlain() gen.ProcessBehavior { return &pluPlain{} }

func (p *pluPlain) Init(args ...any) (act.PoolOptions, error) {
	return args[0].(act.PoolOptions), nil
}

func TestPoolUnitDefaultCallbacks(t *testing.T) {
	s, err := unit.Spawn(t, factoryPluPlain, gen.ProcessOptions{}, poolOpts(1))
	check.NoError(t, err)
	s.SendMessageWithPriority(gen.PID{}, "m", gen.MessagePriorityHigh) // default HandleMessage (warn)
	resp, err := s.CallWithPriority(gen.PID{}, "q", gen.MessagePriorityHigh) // default HandleCall (warn, nil)
	check.NoError(t, err)
	check.Nil(t, resp)
	m, err := s.Inspect(gen.PID{}) // default HandleInspect (stats)
	check.NoError(t, err)
	check.NotNil(t, m)
	s.ShouldTerminate().None().Assert()
}

type pluPanic struct{ act.Pool }

func factoryPluPanic() gen.ProcessBehavior { return &pluPanic{} }

func (p *pluPanic) Init(args ...any) (act.PoolOptions, error) { panic("pool init boom") }
