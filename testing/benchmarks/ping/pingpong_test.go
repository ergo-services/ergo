// Package ping measures message throughput between processes: one sender to one
// receiver, and one pair per CPU, both on a single node and across a network
// connection between two nodes; the round trip of one pair (a message there, the
// answer back, the next one after it), on a node and across the network; one
// sender of big messages across the network, each one cut into fragments on the
// way.
//
// What is measured: the window opens before the senders are triggered and
// closes when the last receiver has handled the last message, so the reported
// msg/sec is the rate messages are carried end to end - the send loops
// included, not the rate Send is called at. Each scenario carries b.N messages
// in total, split evenly across the pairs; a round trip scenario makes b.N round
// trips.
//
// Run it with `make bench`, or with an exact message count instead of a
// duration:
//
//	go test -run XXX -bench . -benchmem -benchtime 14000000x ./testing/benchmarks/ping
//
// With BENCH_UNORDERED set the senders do not keep the network order (a TCP of
// the pool and a decoder per message, not per sender and receiver).
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
	echoName gen.Atom = "echo"
	done     gen.Atom = "done"

	// the size of a message of NetworkBig: cut into fragments of
	// gen.DefaultFragmentSize bytes on the way
	big = 1 << 20
)

// unordered: BENCH_UNORDERED is set, the senders do not keep the network order
var unordered = os.Getenv("BENCH_UNORDERED") != ""

type send struct{}

type pingOptions struct {
	remote  gen.Atom
	n       int
	message any

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
	if unordered == true {
		if err := p.SetKeepNetworkOrder(false); err != nil {
			return err
		}
	}

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
			if err := p.SendPID(p.pair, p.options.message); err != nil {
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

func benchmark(b *testing.B, np int, message any, host gen.Node, spawn func(o pingOptions) (gen.PID, error)) {
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
			message:  message,
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
	benchmark(b, np, 1, node, func(o pingOptions) (gen.PID, error) {
		return node.Spawn(factoryPing, gen.ProcessOptions{}, o)
	})
}

// nodes starts two nodes, connected, the second one spawning name for the
// first one.
func nodes(b *testing.B, name gen.Atom, factory gen.ProcessFactory, acceptors ...gen.AcceptorOptions) (gen.Node, gen.Node) {
	nodeping := startNode(b, "net_ping", acceptors...)
	nodepong := startNode(b, "net_pong", acceptors...)

	if err := nodepong.Network().EnableSpawn(name, factory); err != nil {
		b.Fatalf("unable to enable remote spawn: %s", err)
	}
	if _, err := nodeping.Network().GetNode(nodepong.Name()); err != nil {
		b.Fatalf("unable to connect the nodes: %s", err)
	}
	return nodeping, nodepong
}

func network(b *testing.B, np int, message any, acceptors ...gen.AcceptorOptions) {
	nodeping, nodepong := nodes(b, pongName, factoryPong, acceptors...)
	benchmark(b, np, message, nodeping, func(o pingOptions) (gen.PID, error) {
		o.remote = nodepong.Name()
		return nodeping.Spawn(factoryPing, gen.ProcessOptions{}, o)
	})
}

type volleyOptions struct {
	partner  gen.PID
	n        int
	finished chan struct{}
}

func factoryVolley() gen.ProcessBehavior {
	return &volley{}
}

// volley plays round trips with its partner: a message there, the answer
// back, n times; closes finished at the end.
type volley struct {
	act.Actor

	options volleyOptions
	left    int
}

func (v *volley) Init(args ...any) error {
	v.options = args[0].(volleyOptions)
	v.left = v.options.n
	if unordered == true {
		return v.SetKeepNetworkOrder(false)
	}
	return nil
}

func (v *volley) HandleMessage(from gen.PID, message any) error {
	if _, serve := message.(send); serve == false {
		v.left--
		if v.left == 0 {
			close(v.options.finished)
			return nil
		}
	}
	return v.SendPID(v.options.partner, 1)
}

func factoryEcho() gen.ProcessBehavior {
	return &echo{}
}

// echo answers every message to its sender.
type echo struct {
	act.Actor
}

func (e *echo) Init(args ...any) error {
	if unordered == true {
		return e.SetKeepNetworkOrder(false)
	}
	return nil
}

func (e *echo) HandleMessage(from gen.PID, message any) error {
	return e.SendPID(from, 1)
}

// rtt makes b.N round trips between a volley on host and partner.
func rtt(b *testing.B, host gen.Node, partner gen.PID) {
	finished := make(chan struct{})
	pid, err := host.Spawn(factoryVolley, gen.ProcessOptions{}, volleyOptions{
		partner:  partner,
		n:        b.N,
		finished: finished,
	})
	if err != nil {
		b.Fatalf("unable to spawn volley: %s", err)
	}

	b.ResetTimer()
	start := time.Now()
	if err := host.Send(pid, send{}); err != nil {
		b.Fatalf("unable to trigger the volley: %s", err)
	}
	<-finished
	elapsed := time.Since(start)
	b.StopTimer()

	b.ReportMetric(float64(b.N)/elapsed.Seconds(), "rtt/sec")
}

func BenchmarkLocal11(b *testing.B) {
	local(b, 1)
}

func BenchmarkLocalNN(b *testing.B) {
	local(b, runtime.NumCPU())
}

func BenchmarkNetwork11(b *testing.B) {
	network(b, 1, 1)
}

func BenchmarkNetworkNN(b *testing.B) {
	network(b, runtime.NumCPU(), 1, gen.AcceptorOptions{
		Handshake: handshake.Create(handshake.Options{PoolSize: runtime.NumCPU() / 2}),
	})
}

func BenchmarkLocalRtt(b *testing.B) {
	node := startNode(b, "local")
	pid, err := node.Spawn(factoryEcho, gen.ProcessOptions{})
	if err != nil {
		b.Fatalf("unable to spawn echo: %s", err)
	}
	rtt(b, node, pid)
}

func BenchmarkNetworkRtt(b *testing.B) {
	nodeping, nodepong := nodes(b, echoName, factoryEcho)
	remote, err := nodeping.Network().Node(nodepong.Name())
	if err != nil {
		b.Fatalf("unable to get the remote node: %s", err)
	}
	pid, err := remote.Spawn(echoName, gen.ProcessOptions{})
	if err != nil {
		b.Fatalf("unable to spawn echo: %s", err)
	}
	rtt(b, nodeping, pid)
}

func BenchmarkNetworkBig(b *testing.B) {
	payload := make([]byte, big)
	for i := range payload {
		payload[i] = 7
	}
	b.SetBytes(big)
	network(b, 1, payload)
}
