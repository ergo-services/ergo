// Package ping measures message throughput between processes: one sender to one
// receiver, and one pair per CPU, both on a single node and across a network
// connection between two nodes.
//
// What is measured: the window opens before the senders are triggered and
// closes when the last receiver has handled the last message, so the reported
// msg/sec is the rate messages are carried end to end - the send loops
// included, not the rate Send is called at. Each scenario carries b.N messages
// in total, split evenly across the pairs.
//
// Run it with `make bench`, or with an exact message count instead of a
// duration:
//
//	go test -run XXX -bench . -benchmem -benchtime 14000000x ./testing/benchmarks/ping
package ping

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/net/handshake"
)

const (
	pongName gen.Atom = "pong"
	done     gen.Atom = "done"
)

type send struct{}

type pingOptions struct {
	remote gen.Atom
	n      int

	ready    chan struct{}
	finished chan struct{}
}

func factoryPing() gen.ProcessBehavior {
	return &ping{}
}

type ping struct {
	act.Actor

	options pingOptions
	pair    gen.PID
}

func (p *ping) Init(args ...any) error {
	p.options = args[0].(pingOptions)

	if p.options.remote == "" {
		pid, err := p.Spawn(factoryPong, gen.ProcessOptions{}, p.options.n)
		if err != nil {
			return err
		}
		p.pair = pid
	} else {
		remote, err := p.Node().Network().Node(p.options.remote)
		if err != nil {
			return err
		}
		pid, err := remote.Spawn(pongName, gen.ProcessOptions{}, p.options.n)
		if err != nil {
			return err
		}
		p.pair = pid
	}

	close(p.options.ready)
	return nil
}

func (p *ping) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case send:
		for i := 0; i < p.options.n; i++ {
			if err := p.SendPID(p.pair, 1); err != nil {
				return err
			}
		}
		return nil

	case gen.Atom:
		if message == done {
			close(p.options.finished)
		}
		return nil
	}
	return nil
}

func factoryPong() gen.ProcessBehavior {
	return &pong{}
}

type pong struct {
	act.Actor

	target int
	count  int
}

func (p *pong) Init(args ...any) error {
	if len(args) != 1 {
		return gen.ErrIncorrect
	}
	target, ok := args[0].(int)
	if ok == false {
		return gen.ErrIncorrect
	}
	p.target = target
	return nil
}

func (p *pong) HandleMessage(from gen.PID, message any) error {
	p.count++
	if p.count != p.target {
		return nil
	}
	return p.SendPID(from, done)
}

var run atomic.Uint64

func nodeName(tag string) gen.Atom {
	return gen.Atom(fmt.Sprintf("ping_%s_%d_%d@localhost", tag, os.Getpid(), run.Add(1)))
}

func startNode(b *testing.B, tag string, acceptors ...gen.AcceptorOptions) gen.Node {
	b.Helper()

	var options gen.NodeOptions
	options.Network.Cookie = "cookie"
	options.Log.DefaultLogger.Disable = true
	options.Network.Acceptors = acceptors

	node, err := ergo.StartNode(nodeName(tag), options)
	if err != nil {
		b.Fatalf("unable to start node: %s", err)
	}
	b.Cleanup(node.StopForce)
	return node
}

func benchmark(b *testing.B, np int, host gen.Node, spawn func(o pingOptions) (gen.PID, error)) {
	per := b.N / np
	if per == 0 {
		per = 1
	}
	total := per * np

	pids := make([]gen.PID, np)
	options := make([]pingOptions, np)

	for i := 0; i < np; i++ {
		options[i] = pingOptions{
			n:        per,
			ready:    make(chan struct{}),
			finished: make(chan struct{}),
		}
		pid, err := spawn(options[i])
		if err != nil {
			b.Fatalf("unable to spawn ping: %s", err)
		}
		pids[i] = pid
	}

	for i := range options {
		<-options[i].ready
	}

	b.ResetTimer()
	start := time.Now()

	for i := range pids {
		if err := host.Send(pids[i], send{}); err != nil {
			b.Fatalf("unable to trigger the sender: %s", err)
		}
	}
	for i := range options {
		<-options[i].finished
	}

	elapsed := time.Since(start)
	b.StopTimer()

	b.ReportMetric(float64(total)/elapsed.Seconds(), "msg/sec")
}

func local(b *testing.B, np int) {
	node := startNode(b, "local")
	benchmark(b, np, node, func(o pingOptions) (gen.PID, error) {
		return node.Spawn(factoryPing, gen.ProcessOptions{}, o)
	})
}

func network(b *testing.B, np int, acceptors ...gen.AcceptorOptions) {
	nodeping := startNode(b, "net_ping", acceptors...)
	nodepong := startNode(b, "net_pong", acceptors...)

	if err := nodepong.Network().EnableSpawn(pongName, factoryPong); err != nil {
		b.Fatalf("unable to enable remote spawn: %s", err)
	}
	if _, err := nodeping.Network().GetNode(nodepong.Name()); err != nil {
		b.Fatalf("unable to connect the nodes: %s", err)
	}

	benchmark(b, np, nodeping, func(o pingOptions) (gen.PID, error) {
		o.remote = nodepong.Name()
		return nodeping.Spawn(factoryPing, gen.ProcessOptions{}, o)
	})
}

func BenchmarkLocal11(b *testing.B) {
	local(b, 1)
}

func BenchmarkLocalNN(b *testing.B) {
	local(b, runtime.NumCPU())
}

func BenchmarkNetwork11(b *testing.B) {
	network(b, 1)
}

func BenchmarkNetworkNN(b *testing.B) {
	network(b, runtime.NumCPU(), gen.AcceptorOptions{
		Handshake: handshake.Create(handshake.Options{PoolSize: runtime.NumCPU() / 2}),
	})
}
