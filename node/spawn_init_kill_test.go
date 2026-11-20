package node

import (
	"sync/atomic"
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

// initKiller kills itself from within ProcessInit, so spawn leaves the Init state
// with a kill already pending.
type initKiller struct {
	proc       gen.Process
	ran        int32
	terminated int32
}

func (b *initKiller) ProcessInit(process gen.Process, args ...any) error {
	b.proc = process
	return process.Node().Kill(process.PID())
}

func (b *initKiller) ProcessRun() error {
	atomic.AddInt32(&b.ran, 1)
	return nil
}

func (b *initKiller) ProcessTerminate(reason error) {
	atomic.AddInt32(&b.terminated, 1)
}

func (b *initKiller) ProcessKind() gen.ProcessKind { return gen.ProcessKindCustom }

func startTestNode(t *testing.T, name gen.Atom) gen.Node {
	t.Helper()
	n, err := Start(name, NodeOptionsExtra{
		NodeOptions: gen.NodeOptions{
			Network: gen.NetworkOptions{Mode: gen.NetworkModeDisabled},
			Log: gen.LogOptions{
				DefaultLogger: gen.DefaultLoggerOptions{Disable: true},
			},
		},
	})
	if err != nil {
		t.Fatalf("unable to start the node: %s", err)
	}
	t.Cleanup(func() { n.StopForce() })
	return n
}

// spawnKilledDuringInit spawns a process that kills itself in Init and waits for its
// terminate callback.
func spawnKilledDuringInit(t *testing.T, n gen.Node) *initKiller {
	t.Helper()
	b := &initKiller{}
	if _, err := n.Spawn(func() gen.ProcessBehavior { return b }, gen.ProcessOptions{}); err != nil {
		t.Fatalf("unable to spawn: %s", err)
	}
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if atomic.LoadInt32(&b.terminated) > 0 {
			return b
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the process killed during Init was not terminated")
	return nil
}

// A kill that lands while the process is still in Init leaves it terminated, and the
// run loop never starts.
func TestSpawnKillDuringInitState(t *testing.T) {
	n := startTestNode(t, "spawn_kill_init@localhost")

	b := spawnKilledDuringInit(t, n)

	if state := b.proc.State(); state != gen.ProcessStateTerminated {
		t.Fatalf("the killed process is in %s state, expected terminated", state)
	}
	if ran := atomic.LoadInt32(&b.ran); ran != 0 {
		t.Fatalf("the killed process ran %d time(s)", ran)
	}
}

// The terminate path taken on a kill during Init keeps the shutdown barrier balanced,
// so the node is still able to stop.
func TestSpawnKillDuringInitShutdownBarrier(t *testing.T) {
	n := startTestNode(t, "spawn_kill_init_barrier@localhost")

	spawnKilledDuringInit(t, n)

	released := make(chan struct{})
	go func() {
		n.(*node).waitprocesses.Wait()
		close(released)
	}()

	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("the shutdown barrier was not released by the killed process")
	}
}
