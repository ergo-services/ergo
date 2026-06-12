// Package stage is the live multi-node system-test harness. It starts real
// nodes, drives them through the public gen.Node API, and observes real
// happenings by decorating each node's routing surface (gen.Core, ingress) and
// each process's gen.Process (egress) via node.NodeOptionsExtra. Assertions use
// the shared testing/check grammar. Unlike testing/unit (mock node, single
// actor), stage runs the actual runtime.
package stage

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ergo.services/ergo/app/system"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/node"
	"ergo.services/ergo/testing/check"
)

// frameworkVersion is recorded into started nodes. Stage tests do not depend on
// the real framework version, so a fixed stage marker is enough.
var frameworkVersion = gen.Version{Name: "ergo.services/stage", Release: "test"}

var stageSeq atomic.Uint64

// Stage owns a set of live nodes and tears them down on test cleanup.
type Stage struct {
	t     *testing.T
	id    uint64
	mu    sync.Mutex
	nodes []*Node
}

// New creates a stage and registers teardown via t.Cleanup.
func New(t *testing.T) *Stage {
	s := &Stage{t: t, id: stageSeq.Add(1)}
	t.Cleanup(s.stop)
	return s
}

func (s *Stage) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.nodes) - 1; i >= 0; i-- {
		if n := s.nodes[i]; n.node != nil && n.node.IsAlive() {
			n.node.StopForce()
		}
	}
}

// NodeOptions configures a stage node.
type NodeOptions struct {
	Applications []gen.ApplicationBehavior
	Env          map[gen.Env]any
	Cookie       string
	// EnableSystemApp starts the node with the system application. By default a
	// stage node is bare (no processes), so tests can assert exact process and
	// application counts; set this only when the test needs system services.
	EnableSystemApp bool

	// Distributed network options (pass-through to gen.NodeOptions.Network).
	// Zero values keep the framework defaults.
	MaxMessageSize int
	FragmentSize   int
	NetworkFlags   gen.NetworkFlags

	// Security pass-through (e.g. ExposeEnvRemoteSpawn for remote-spawn env inheritance).
	Security gen.SecurityOptions
}

// Node is a live node started by the stage, with its per-node recorder.
type Node struct {
	s    *Stage
	t    *testing.T
	node gen.Node
	rec  *recorder
}

// Node starts a live node. Names are unique per stage to avoid collisions.
func (s *Stage) Node(name string, opts ...NodeOptions) *Node {
	s.t.Helper()
	var o NodeOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	cookie := o.Cookie
	if cookie == "" {
		cookie = "stage"
	}

	no := gen.NodeOptions{}
	no.Log.DefaultLogger.Disable = true
	no.Network.Cookie = cookie
	no.Network.MaxMessageSize = o.MaxMessageSize
	no.Network.FragmentSize = o.FragmentSize
	no.Network.Flags = o.NetworkFlags
	no.Security = o.Security
	no.Env = o.Env
	if o.EnableSystemApp {
		no.Applications = append([]gen.ApplicationBehavior{system.CreateApp()}, o.Applications...)
	} else {
		no.Applications = o.Applications
	}

	// unique across parallel test processes (pid) and within a process (seq)
	r := &recorder{q: lib.NewQueueMPSC()}
	nodeName := gen.Atom(fmt.Sprintf("%s-stage-%d-%d@localhost", name, os.Getpid(), s.id))
	gn, err := node.Start(nodeName, node.NodeOptionsExtra{
		NodeOptions:      no,
		FrameworkVersion: frameworkVersion,
		WrapCore:         func(c gen.Core) gen.Core { return &recordCore{Core: c, rec: r} },
		WrapProcess:      func(p gen.Process) gen.Process { return &recordProcess{Process: p, rec: r} },
		WrapCoreTargetManager: func(b gen.CoreTargetManager) gen.CoreTargetManager {
			return &recordBridge{CoreTargetManager: b, rec: r}
		},
	})
	if err != nil {
		s.t.Fatalf("stage: start node %s: %s", nodeName, err)
	}

	n := &Node{s: s, t: s.t, node: gn, rec: r}
	s.mu.Lock()
	s.nodes = append(s.nodes, n)
	s.mu.Unlock()
	return n
}

// Name returns the node's full name.
func (n *Node) Name() gen.Atom { return n.node.Name() }

// PID returns the node's core PID.
func (n *Node) PID() gen.PID { return n.node.PID() }

// Native returns the underlying gen.Node.
func (n *Node) Native() gen.Node { return n.node }

// Mark returns the current recorder position. Pass it to an assertion's
// Since(mark) to scope matching to records observed after this point.
func (n *Node) Mark() int { return len(n.rec.Records()) }

// ProcessPID returns the PID registered under name on this node, as the node
// reports it.
func (n *Node) ProcessPID(name gen.Atom) (gen.PID, error) { return n.node.ProcessPID(name) }

// Spawn spawns a process on the node, failing the test on error. Mirrors
// gen.Node.Spawn (factory, options, args...); pass gen.ProcessOptions{} for
// defaults.
func (n *Node) Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) gen.PID {
	n.t.Helper()
	pid, err := n.node.Spawn(factory, options, args...)
	if err != nil {
		n.t.Fatalf("stage: spawn on %s: %s", n.node.Name(), err)
	}
	return pid
}

// SpawnRegister spawns a named process on the node, failing the test on error.
// Mirrors gen.Node.SpawnRegister (register, factory, options, args...).
func (n *Node) SpawnRegister(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) gen.PID {
	n.t.Helper()
	pid, err := n.node.SpawnRegister(register, factory, options, args...)
	if err != nil {
		n.t.Fatalf("stage: spawn %s on %s: %s", register, n.node.Name(), err)
	}
	return pid
}

// Send delivers a message to a target (node-level send, sender is the node).
func (n *Node) Send(to any, message any) {
	n.t.Helper()
	if err := n.node.Send(to, message); err != nil {
		n.t.Fatalf("stage: send to %v: %s", to, err)
	}
}

// Call makes a synchronous request to a target (node-level, sender is the node).
func (n *Node) Call(to any, request any) (any, error) { return n.node.Call(to, request) }

// SendExit sends an exit signal to a process (node-level, sender is the node).
func (n *Node) SendExit(pid gen.PID, reason error) error { return n.node.SendExit(pid, reason) }

// ProcessID builds a ProcessID addressing the named process on this node. Handy
// for cross-node addressing: n1.Call(n2.ProcessID("svc"), req).
func (n *Node) ProcessID(name gen.Atom) gen.ProcessID {
	return gen.ProcessID{Name: name, Node: n.node.Name()}
}

// EnableSpawn allows remote nodes to spawn the named factory on this node
// (optionally restricting to the given node names).
func (n *Node) EnableSpawn(name gen.Atom, factory gen.ProcessFactory, nodes ...gen.Atom) {
	n.t.Helper()
	if err := n.node.Network().EnableSpawn(name, factory, nodes...); err != nil {
		n.t.Fatalf("stage: EnableSpawn %s on %s: %s", name, n.node.Name(), err)
	}
}

// EnableApplicationStart allows remote nodes to start the named application on
// this node (optionally restricting to the given node names).
func (n *Node) EnableApplicationStart(name gen.Atom, nodes ...gen.Atom) {
	n.t.Helper()
	if err := n.node.Network().EnableApplicationStart(name, nodes...); err != nil {
		n.t.Fatalf("stage: EnableApplicationStart %s on %s: %s", name, n.node.Name(), err)
	}
}

// Connect establishes a connection from a to b and waits until both sides have
// registered it, then returns the RemoteNode from a's perspective. The wait is
// deterministic (polls for the reverse registration rather than sleeping).
func (s *Stage) Connect(a, b *Node) gen.RemoteNode {
	s.t.Helper()
	remote, err := a.node.Network().GetNode(b.node.Name())
	if err != nil {
		s.t.Fatalf("stage: connect %s -> %s: %s", a.node.Name(), b.node.Name(), err)
	}
	// b registers the reverse connection asynchronously; wait for it
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := b.node.Network().Node(a.node.Name()); err == nil {
			break
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("stage: connect %s <- %s: peer did not register the connection in time",
				a.node.Name(), b.node.Name())
		}
		time.Sleep(5 * time.Millisecond)
	}
	return remote
}

// ConnectMesh dials every ordered pair of nodes concurrently, so each link is
// attempted from both sides at once and exercises simultaneous-connect collision
// resolution. It then waits until every node sees every other, re-dialing any
// missing pair until the mesh settles or the deadline passes (a real cluster
// retries dials that the TCP backlog dropped under a connect storm). Duplicate
// detection is left to the caller via Network().Nodes().
func (s *Stage) ConnectMesh(nodes ...*Node) {
	s.t.Helper()
	var wg sync.WaitGroup
	for i := range nodes {
		for j := range nodes {
			if i == j {
				continue
			}
			wg.Add(1)
			go func(src, dst *Node) {
				defer wg.Done()
				src.node.Network().GetNode(dst.node.Name())
			}(nodes[i], nodes[j])
		}
	}
	wg.Wait()

	deadline := time.Now().Add(20 * time.Second)
	for {
		missing := 0
		for i := range nodes {
			for j := range nodes {
				if i == j {
					continue
				}
				if _, err := nodes[i].node.Network().Node(nodes[j].node.Name()); err != nil {
					missing++
					nodes[i].node.Network().GetNode(nodes[j].node.Name())
				}
			}
		}
		if missing == 0 {
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("stage: mesh of %d nodes did not settle: %d missing connections", len(nodes), missing)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Kill force-terminates a process (TerminateReasonKill).
func (s *Stage) Kill(n *Node, pid gen.PID) {
	s.t.Helper()
	if err := n.node.Kill(pid); err != nil {
		s.t.Fatalf("stage: kill %s: %s", pid, err)
	}
}

// recorder: per-node sink, a check.Source
//
// Producers (actor loops, the routing core, connections) push concurrently via
// put; the queue is multi-producer/single-consumer. The consumer side (Records,
// drained by Mark and the assertion engine) is single-goroutine: a Node's Mark /
// Should* / assertions must all be called from one goroutine (the test goroutine).
// Test-level concurrency that only drives the node API (e.g. ConnectMesh dialing)
// is fine; concurrent assertions on the same node are not supported.
type recorder struct {
	q      lib.QueueMPSC
	stored []check.Record
}

func (r *recorder) put(rec check.Record) { r.q.Push(rec) }

// Records drains the queue into the append-only history and returns it. The slice
// is shared (no copy): the history only grows, callers only read it, and the
// single-consumer contract means no concurrent mutation.
func (r *recorder) Records() []check.Record {
	for {
		v, ok := r.q.Pop()
		if ok == false {
			break
		}
		r.stored = append(r.stored, v.(check.Record))
	}
	return r.stored
}

// recordCore: ingress on the routing surface (gen.Core)
// Two kinds of happening cross this surface: a message delivered into a local
// mailbox (the egress counterpart is Sent), and a remote subscription arriving
// over the wire (the egress counterpart is Linked/Monitored).

type recordCore struct {
	gen.Core
	rec *recorder
}

func (c *recordCore) local(node gen.Atom) bool {
	return node == "" || node == c.Core.Name()
}

func (c *recordCore) RouteSendPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	err := c.Core.RouteSendPID(from, to, options, message)
	if err == nil && c.local(to.Node) {
		c.rec.put(Delivered{From: from, To: to, Message: message})
	}
	return err
}

func (c *recordCore) RouteSendProcessID(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error {
	err := c.Core.RouteSendProcessID(from, to, options, message)
	if err == nil && c.local(to.Node) {
		c.rec.put(Delivered{From: from, To: to, Message: message})
	}
	return err
}

func (c *recordCore) RouteCallPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	err := c.Core.RouteCallPID(from, to, options, message)
	if err == nil && c.local(to.Node) {
		c.rec.put(Delivered{From: from, To: to, Message: message})
	}
	return err
}

func (c *recordCore) RouteCallProcessID(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error {
	err := c.Core.RouteCallProcessID(from, to, options, message)
	if err == nil && c.local(to.Node) {
		c.rec.put(Delivered{From: from, To: to, Message: message})
	}
	return err
}

func (c *recordCore) RouteSendAlias(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error {
	err := c.Core.RouteSendAlias(from, to, options, message)
	if err == nil && c.local(to.Node) {
		c.rec.put(Delivered{From: from, To: to, Message: message})
	}
	return err
}

func (c *recordCore) RouteCallAlias(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error {
	err := c.Core.RouteCallAlias(from, to, options, message)
	if err == nil && c.local(to.Node) {
		c.rec.put(Delivered{From: from, To: to, Message: message})
	}
	return err
}

func (c *recordCore) RouteSendExit(from gen.PID, to gen.PID, reason error) error {
	err := c.Core.RouteSendExit(from, to, reason)
	if err == nil && c.local(to.Node) {
		c.rec.put(Exit{To: to, Message: gen.MessageExitPID{PID: from, Reason: reason}})
	}
	return err
}

// Wire-level subscription ingress: a remote consumer's link/monitor (or its
// removal) arriving over the connection. The sender-side manager deduplicates, so
// these record exactly the subscriptions that crossed the wire: one per remote
// node regardless of how many local subscribers it has, removed only when its last
// local subscriber leaves. Only remote consumers are recorded (local subscriptions
// never reach gen.Core's route methods over the wire).

func (c *recordCore) RouteLinkPID(pid gen.PID, target gen.PID) error {
	err := c.Core.RouteLinkPID(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireLink{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteUnlinkPID(pid gen.PID, target gen.PID) error {
	err := c.Core.RouteUnlinkPID(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireUnlink{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteLinkProcessID(pid gen.PID, target gen.ProcessID) error {
	err := c.Core.RouteLinkProcessID(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireLink{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteUnlinkProcessID(pid gen.PID, target gen.ProcessID) error {
	err := c.Core.RouteUnlinkProcessID(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireUnlink{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteLinkAlias(pid gen.PID, target gen.Alias) error {
	err := c.Core.RouteLinkAlias(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireLink{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteUnlinkAlias(pid gen.PID, target gen.Alias) error {
	err := c.Core.RouteUnlinkAlias(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireUnlink{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteLinkEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {
	r, err := c.Core.RouteLinkEvent(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireLink{From: pid, Target: target})
	}
	return r, err
}

func (c *recordCore) RouteUnlinkEvent(pid gen.PID, target gen.Event) error {
	err := c.Core.RouteUnlinkEvent(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireUnlink{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteMonitorPID(pid gen.PID, target gen.PID) error {
	err := c.Core.RouteMonitorPID(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireMonitor{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteDemonitorPID(pid gen.PID, target gen.PID) error {
	err := c.Core.RouteDemonitorPID(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireDemonitor{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteMonitorProcessID(pid gen.PID, target gen.ProcessID) error {
	err := c.Core.RouteMonitorProcessID(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireMonitor{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteDemonitorProcessID(pid gen.PID, target gen.ProcessID) error {
	err := c.Core.RouteDemonitorProcessID(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireDemonitor{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteMonitorAlias(pid gen.PID, target gen.Alias) error {
	err := c.Core.RouteMonitorAlias(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireMonitor{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteDemonitorAlias(pid gen.PID, target gen.Alias) error {
	err := c.Core.RouteDemonitorAlias(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireDemonitor{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteMonitorEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {
	r, err := c.Core.RouteMonitorEvent(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireMonitor{From: pid, Target: target})
	}
	return r, err
}

func (c *recordCore) RouteDemonitorEvent(pid gen.PID, target gen.Event) error {
	err := c.Core.RouteDemonitorEvent(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.put(WireDemonitor{From: pid, Target: target})
	}
	return err
}

// recordBridge: reception of Down/Exit/Event delivered by the target manager
// (these bypass gen.Core; the target manager bridge is their delivery surface)

type recordBridge struct {
	gen.CoreTargetManager
	rec *recorder
}

func (b *recordBridge) RouteSendPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	err := b.CoreTargetManager.RouteSendPID(from, to, options, message)
	switch message.(type) {
	case gen.MessageDownPID, gen.MessageDownProcessID, gen.MessageDownAlias, gen.MessageDownNode, gen.MessageDownEvent:
		b.rec.put(Down{To: to, Message: message})
	case gen.MessageEventStart, gen.MessageEventStop:
		// producer notifications about first subscriber / last unsubscribe
		b.rec.put(Delivered{From: from, To: to, Message: message})
	}
	return err
}

func (b *recordBridge) RouteSendExitMessages(from gen.PID, to []gen.PID, message any) error {
	err := b.CoreTargetManager.RouteSendExitMessages(from, to, message)
	for _, pid := range to {
		b.rec.put(Exit{To: pid, Message: message})
	}
	return err
}

func (b *recordBridge) RouteSendEventMessages(from gen.PID, to []gen.PID, options gen.MessageOptions, message gen.MessageEvent) error {
	err := b.CoreTargetManager.RouteSendEventMessages(from, to, options, message)
	for _, pid := range to {
		b.rec.put(Event{To: pid, Event: message.Event, Timestamp: message.Timestamp, Message: message.Message})
	}
	return err
}

// recordProcess: egress (records a process's outgoing actions)
type recordProcess struct {
	gen.Process
	rec *recorder
}

func (p *recordProcess) Monitor(target any) error {
	err := p.Process.Monitor(target)
	p.rec.put(Monitored{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) MonitorPID(pid gen.PID) error {
	err := p.Process.MonitorPID(pid)
	p.rec.put(Monitored{From: p.Process.PID(), Target: pid, Error: err})
	return err
}

func (p *recordProcess) MonitorProcessID(target gen.ProcessID) error {
	err := p.Process.MonitorProcessID(target)
	p.rec.put(Monitored{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) MonitorAlias(target gen.Alias) error {
	err := p.Process.MonitorAlias(target)
	p.rec.put(Monitored{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) MonitorEvent(target gen.Event) ([]gen.MessageEvent, error) {
	last, err := p.Process.MonitorEvent(target)
	p.rec.put(Monitored{From: p.Process.PID(), Target: target, Error: err})
	return last, err
}

func (p *recordProcess) Demonitor(target any) error {
	err := p.Process.Demonitor(target)
	p.rec.put(Demonitored{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) DemonitorPID(pid gen.PID) error {
	err := p.Process.DemonitorPID(pid)
	p.rec.put(Demonitored{From: p.Process.PID(), Target: pid, Error: err})
	return err
}

func (p *recordProcess) DemonitorProcessID(target gen.ProcessID) error {
	err := p.Process.DemonitorProcessID(target)
	p.rec.put(Demonitored{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) DemonitorAlias(target gen.Alias) error {
	err := p.Process.DemonitorAlias(target)
	p.rec.put(Demonitored{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) DemonitorEvent(target gen.Event) error {
	err := p.Process.DemonitorEvent(target)
	p.rec.put(Demonitored{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) Link(target any) error {
	err := p.Process.Link(target)
	p.rec.put(Linked{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) LinkPID(pid gen.PID) error {
	err := p.Process.LinkPID(pid)
	p.rec.put(Linked{From: p.Process.PID(), Target: pid, Error: err})
	return err
}

func (p *recordProcess) LinkProcessID(target gen.ProcessID) error {
	err := p.Process.LinkProcessID(target)
	p.rec.put(Linked{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) LinkAlias(target gen.Alias) error {
	err := p.Process.LinkAlias(target)
	p.rec.put(Linked{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) LinkEvent(target gen.Event) ([]gen.MessageEvent, error) {
	last, err := p.Process.LinkEvent(target)
	p.rec.put(Linked{From: p.Process.PID(), Target: target, Error: err})
	return last, err
}

func (p *recordProcess) Unlink(target any) error {
	err := p.Process.Unlink(target)
	p.rec.put(Unlinked{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) UnlinkPID(pid gen.PID) error {
	err := p.Process.UnlinkPID(pid)
	p.rec.put(Unlinked{From: p.Process.PID(), Target: pid, Error: err})
	return err
}

func (p *recordProcess) UnlinkProcessID(target gen.ProcessID) error {
	err := p.Process.UnlinkProcessID(target)
	p.rec.put(Unlinked{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) UnlinkAlias(target gen.Alias) error {
	err := p.Process.UnlinkAlias(target)
	p.rec.put(Unlinked{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) UnlinkEvent(target gen.Event) error {
	err := p.Process.UnlinkEvent(target)
	p.rec.put(Unlinked{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) SendEvent(name gen.Atom, token gen.Ref, message any) error {
	err := p.Process.SendEvent(name, token, message)
	p.rec.put(SentEvent{From: p.Process.PID(), Name: name, Message: message, Error: err})
	return err
}

func (p *recordProcess) SendResponse(to gen.PID, ref gen.Ref, message any) error {
	err := p.Process.SendResponse(to, ref, message)
	p.rec.put(SentResponse{From: p.Process.PID(), To: to, Ref: ref, Message: message, Error: err})
	return err
}

func (p *recordProcess) SendResponseImportant(to gen.PID, ref gen.Ref, message any) error {
	err := p.Process.SendResponseImportant(to, ref, message)
	p.rec.put(SentResponse{From: p.Process.PID(), To: to, Ref: ref, Message: message, Error: err})
	return err
}

func (p *recordProcess) SendResponseError(to gen.PID, ref gen.Ref, e error) error {
	err := p.Process.SendResponseError(to, ref, e)
	p.rec.put(SentResponse{From: p.Process.PID(), To: to, Ref: ref, Message: e, Error: err})
	return err
}

func (p *recordProcess) SendResponseErrorImportant(to gen.PID, ref gen.Ref, e error) error {
	err := p.Process.SendResponseErrorImportant(to, ref, e)
	p.rec.put(SentResponse{From: p.Process.PID(), To: to, Ref: ref, Message: e, Error: err})
	return err
}

func (p *recordProcess) Send(to any, message any) error {
	err := p.Process.Send(to, message)
	p.rec.put(Sent{From: p.Process.PID(), To: to, Message: message, Error: err})
	return err
}

func (p *recordProcess) SendPID(to gen.PID, message any) error {
	err := p.Process.SendPID(to, message)
	p.rec.put(Sent{From: p.Process.PID(), To: to, Message: message, Error: err})
	return err
}

func (p *recordProcess) SendProcessID(to gen.ProcessID, message any) error {
	err := p.Process.SendProcessID(to, message)
	p.rec.put(Sent{From: p.Process.PID(), To: to, Message: message, Error: err})
	return err
}

func (p *recordProcess) Call(to any, request any) (any, error) {
	response, err := p.Process.Call(to, request)
	p.rec.put(Called{From: p.Process.PID(), To: to, Request: request, Response: response, Error: err})
	return response, err
}

func (p *recordProcess) Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	pid, err := p.Process.Spawn(factory, options, args...)
	p.rec.put(Spawned{Parent: p.Process.PID(), Child: pid, Error: err})
	return pid, err
}

func (p *recordProcess) Forward(to gen.PID, message *gen.MailboxMessage, priority gen.MessagePriority) error {
	// copy the fields before delegating: once forwarded, the message lives in the
	// target mailbox and may be processed/released concurrently.
	from, msg := message.From, message.Message
	err := p.Process.Forward(to, message, priority)
	p.rec.put(Forwarded{By: p.Process.PID(), To: to, From: from, Message: msg, Error: err})
	return err
}

// records

// Sent is an outgoing message observed at the sender (egress). Error is the
// outcome of the send call (nil on success).
type Sent struct {
	From    gen.PID
	To      any
	Message any
	Error   error
}

func (Sent) Kind() string { return "sent" }
func (r Sent) String() string {
	return fmt.Sprintf("Sent(from=%s to=%v msg=%#v err=%v)", r.From, r.To, r.Message, r.Error)
}

// Called is an outgoing request observed at the caller (egress). Error is the
// outcome of the call (nil on success); Response is the value returned.
type Called struct {
	From     gen.PID
	To       any
	Request  any
	Response any
	Error    error
}

func (Called) Kind() string { return "called" }
func (r Called) String() string {
	return fmt.Sprintf("Called(from=%s to=%v req=%#v err=%v)", r.From, r.To, r.Request, r.Error)
}

// Spawned is a child process created (or attempted) by a process (egress). On
// failure Child is the zero PID and Error is set.
type Spawned struct {
	Parent gen.PID
	Child  gen.PID
	Error  error
}

func (Spawned) Kind() string { return "spawned" }
func (r Spawned) String() string {
	return fmt.Sprintf("Spawned(parent=%s child=%s err=%v)", r.Parent, r.Child, r.Error)
}

// Forwarded is a message handed (or attempted) to another process via Forward,
// observed at the forwarder (egress). Used by act.Pool (round-robin) and
// act.Router (by-name routing). By is the forwarder, To the target, From the
// original sender; Error is the outcome of the forward.
type Forwarded struct {
	By      gen.PID
	To      gen.PID
	From    gen.PID
	Message any
	Error   error
}

func (Forwarded) Kind() string { return "forwarded" }
func (r Forwarded) String() string {
	return fmt.Sprintf("Forwarded(by=%s to=%s from=%s msg=%#v err=%v)", r.By, r.To, r.From, r.Message, r.Error)
}

// Delivered is a message delivered into a local mailbox on this node (ingress).
// Down/Exit/Event signals have their own records (Down/Exit/Event), not Delivered.
// Producer notifications (gen.MessageEventStart / MessageEventStop) do surface here
// as Delivered, since they reach the producer as ordinary mailbox messages.
type Delivered struct {
	From    gen.PID
	To      any
	Message any
}

func (Delivered) Kind() string { return "delivered" }
func (r Delivered) String() string {
	return fmt.Sprintf("Delivered(from=%s to=%v msg=%#v)", r.From, r.To, r.Message)
}

// Down is a down notification delivered to a monitoring process (ingress).
// Message is one of gen.MessageDownPID / MessageDownProcessID / etc.
type Down struct {
	To      gen.PID
	Message any
}

func (Down) Kind() string     { return "down" }
func (r Down) String() string { return fmt.Sprintf("Down(to=%s msg=%#v)", r.To, r.Message) }

// Exit is an exit signal delivered to a linked process (ingress).
// Message is one of gen.MessageExitPID / MessageExitProcessID / etc.
type Exit struct {
	To      gen.PID
	Message any
}

func (Exit) Kind() string     { return "exit" }
func (r Exit) String() string { return fmt.Sprintf("Exit(to=%s msg=%#v)", r.To, r.Message) }

// Event is a pub/sub event delivered to a subscriber (ingress).
type Event struct {
	To        gen.PID
	Event     gen.Event
	Timestamp int64
	Message   any
}

func (Event) Kind() string { return "event" }
func (r Event) String() string {
	return fmt.Sprintf("Event(to=%s %s msg=%#v)", r.To, r.Event, r.Message)
}

// Monitored is a monitor set up (or attempted) by a process (egress).
type Monitored struct {
	From   gen.PID
	Target any
	Error  error
}

func (Monitored) Kind() string { return "monitored" }
func (r Monitored) String() string {
	return fmt.Sprintf("Monitored(from=%s target=%v err=%v)", r.From, r.Target, r.Error)
}

// Demonitored is a monitor removed (or attempted) by a process (egress).
type Demonitored struct {
	From   gen.PID
	Target any
	Error  error
}

func (Demonitored) Kind() string { return "demonitored" }
func (r Demonitored) String() string {
	return fmt.Sprintf("Demonitored(from=%s target=%v err=%v)", r.From, r.Target, r.Error)
}

// Linked is a link set up (or attempted) by a process (egress).
type Linked struct {
	From   gen.PID
	Target any
	Error  error
}

func (Linked) Kind() string { return "linked" }
func (r Linked) String() string {
	return fmt.Sprintf("Linked(from=%s target=%v err=%v)", r.From, r.Target, r.Error)
}

// Unlinked is a link removed (or attempted) by a process (egress).
type Unlinked struct {
	From   gen.PID
	Target any
	Error  error
}

func (Unlinked) Kind() string { return "unlinked" }
func (r Unlinked) String() string {
	return fmt.Sprintf("Unlinked(from=%s target=%v err=%v)", r.From, r.Target, r.Error)
}

// WireLink is a remote consumer's link arriving over the connection (ingress on
// the target node). The sender deduplicates, so one per remote node.
type WireLink struct {
	From   gen.PID
	Target any
}

func (WireLink) Kind() string { return "wire_link" }
func (r WireLink) String() string {
	return fmt.Sprintf("WireLink(from=%s target=%v)", r.From, r.Target)
}

// WireUnlink is a remote consumer's unlink arriving over the connection (sent only
// when its last local subscriber leaves).
type WireUnlink struct {
	From   gen.PID
	Target any
}

func (WireUnlink) Kind() string { return "wire_unlink" }
func (r WireUnlink) String() string {
	return fmt.Sprintf("WireUnlink(from=%s target=%v)", r.From, r.Target)
}

// WireMonitor is a remote consumer's monitor arriving over the connection.
type WireMonitor struct {
	From   gen.PID
	Target any
}

func (WireMonitor) Kind() string { return "wire_monitor" }
func (r WireMonitor) String() string {
	return fmt.Sprintf("WireMonitor(from=%s target=%v)", r.From, r.Target)
}

// WireDemonitor is a remote consumer's demonitor arriving over the connection.
type WireDemonitor struct {
	From   gen.PID
	Target any
}

func (WireDemonitor) Kind() string { return "wire_demonitor" }
func (r WireDemonitor) String() string {
	return fmt.Sprintf("WireDemonitor(from=%s target=%v)", r.From, r.Target)
}

// SentEvent is an event published by a process (egress). Error is the outcome of
// the publish (nil on success).
type SentEvent struct {
	From    gen.PID
	Name    gen.Atom
	Message any
	Error   error
}

func (SentEvent) Kind() string { return "sent_event" }
func (r SentEvent) String() string {
	return fmt.Sprintf("SentEvent(from=%s name=%s msg=%#v err=%v)", r.From, r.Name, r.Message, r.Error)
}

// SentResponse is a response a process sent back to a caller's request (egress).
// Error is the outcome of the send call (nil on success); for an error response
// (SendResponseError) the responded error is carried in Message.
type SentResponse struct {
	From    gen.PID
	To      gen.PID
	Ref     gen.Ref
	Message any
	Error   error
}

func (SentResponse) Kind() string { return "sent_response" }
func (r SentResponse) String() string {
	return fmt.Sprintf("SentResponse(from=%s to=%s msg=%#v err=%v)", r.From, r.To, r.Message, r.Error)
}

// fluent assertions (thin wrappers over check.For)

// SentAssert asserts over outgoing messages observed on a node.
type SentAssert struct{ *check.Assertion[Sent] }

// ShouldSend starts an egress message assertion on this node.
func (n *Node) ShouldSend() *SentAssert { return &SentAssert{check.For[Sent](n.t, n.rec)} }
func (a *SentAssert) From(p gen.PID) *SentAssert {
	a.Where(func(r Sent) bool { return r.From == p })
	return a
}
func (a *SentAssert) To(to any) *SentAssert {
	a.Where(func(r Sent) bool { return reflect.DeepEqual(r.To, to) })
	return a
}
func (a *SentAssert) Message(v any) *SentAssert {
	a.Where(func(r Sent) bool { return reflect.DeepEqual(r.Message, v) })
	return a
}
func (a *SentAssert) Error(target error) *SentAssert {
	a.Where(func(r Sent) bool { return r.Error == target })
	return a
}

// SpawnAssert asserts over child processes spawned on a node (egress).
type SpawnAssert struct{ *check.Assertion[Spawned] }

// ShouldSpawn starts a spawn assertion on this node.
func (n *Node) ShouldSpawn() *SpawnAssert { return &SpawnAssert{check.For[Spawned](n.t, n.rec)} }
func (a *SpawnAssert) From(parent gen.PID) *SpawnAssert {
	a.Where(func(r Spawned) bool { return r.Parent == parent })
	return a
}
func (a *SpawnAssert) Child(pid gen.PID) *SpawnAssert {
	a.Where(func(r Spawned) bool { return r.Child == pid })
	return a
}
func (a *SpawnAssert) Error(target error) *SpawnAssert {
	a.Where(func(r Spawned) bool { return r.Error == target })
	return a
}

// ForwardAssert asserts over messages forwarded by a process (egress).
type ForwardAssert struct{ *check.Assertion[Forwarded] }

// ShouldForward starts a forward assertion on this node.
func (n *Node) ShouldForward() *ForwardAssert {
	return &ForwardAssert{check.For[Forwarded](n.t, n.rec)}
}
func (a *ForwardAssert) By(pid gen.PID) *ForwardAssert {
	a.Where(func(r Forwarded) bool { return r.By == pid })
	return a
}
func (a *ForwardAssert) To(pid gen.PID) *ForwardAssert {
	a.Where(func(r Forwarded) bool { return r.To == pid })
	return a
}
func (a *ForwardAssert) Message(v any) *ForwardAssert {
	a.Where(func(r Forwarded) bool { return reflect.DeepEqual(r.Message, v) })
	return a
}
func (a *ForwardAssert) Error(target error) *ForwardAssert {
	a.Where(func(r Forwarded) bool { return r.Error == target })
	return a
}

// DeliveredAssert asserts over messages delivered into local mailboxes (ingress).
type DeliveredAssert struct{ *check.Assertion[Delivered] }

// ShouldDeliver starts an ingress delivery assertion on this node.
func (n *Node) ShouldDeliver() *DeliveredAssert {
	return &DeliveredAssert{check.For[Delivered](n.t, n.rec)}
}
func (a *DeliveredAssert) From(p gen.PID) *DeliveredAssert {
	a.Where(func(r Delivered) bool { return r.From == p })
	return a
}
func (a *DeliveredAssert) To(pid gen.PID) *DeliveredAssert {
	a.Where(func(r Delivered) bool { t, ok := r.To.(gen.PID); return ok && t == pid })
	return a
}
func (a *DeliveredAssert) ToProcessID(target gen.ProcessID) *DeliveredAssert {
	a.Where(func(r Delivered) bool { t, ok := r.To.(gen.ProcessID); return ok && t == target })
	return a
}
func (a *DeliveredAssert) ToAlias(target gen.Alias) *DeliveredAssert {
	a.Where(func(r Delivered) bool { t, ok := r.To.(gen.Alias); return ok && t == target })
	return a
}
func (a *DeliveredAssert) Message(v any) *DeliveredAssert {
	a.Where(func(r Delivered) bool { return reflect.DeepEqual(r.Message, v) })
	return a
}

// CalledAssert asserts over outgoing requests observed on a node.
type CalledAssert struct{ *check.Assertion[Called] }

// ShouldCall starts an egress call assertion on this node.
func (n *Node) ShouldCall() *CalledAssert { return &CalledAssert{check.For[Called](n.t, n.rec)} }
func (a *CalledAssert) From(p gen.PID) *CalledAssert {
	a.Where(func(r Called) bool { return r.From == p })
	return a
}
func (a *CalledAssert) To(to any) *CalledAssert {
	a.Where(func(r Called) bool { return reflect.DeepEqual(r.To, to) })
	return a
}
func (a *CalledAssert) Request(v any) *CalledAssert {
	a.Where(func(r Called) bool { return reflect.DeepEqual(r.Request, v) })
	return a
}
func (a *CalledAssert) Error(target error) *CalledAssert {
	a.Where(func(r Called) bool { return r.Error == target })
	return a
}

// downReason extracts the reason from any gen.MessageDown* value.
func downReason(m any) (error, bool) {
	switch d := m.(type) {
	case gen.MessageDownPID:
		return d.Reason, true
	case gen.MessageDownProcessID:
		return d.Reason, true
	case gen.MessageDownAlias:
		return d.Reason, true
	case gen.MessageDownEvent:
		return d.Reason, true
	}
	return nil, false
}

// exitReason extracts the reason from any gen.MessageExit* value (no reason for node).
func exitReason(m any) (error, bool) {
	switch e := m.(type) {
	case gen.MessageExitPID:
		return e.Reason, true
	case gen.MessageExitProcessID:
		return e.Reason, true
	case gen.MessageExitAlias:
		return e.Reason, true
	case gen.MessageExitEvent:
		return e.Reason, true
	}
	return nil, false
}

// DownAssert asserts over down notifications received on a node (ingress).
type DownAssert struct{ *check.Assertion[Down] }

// ShouldReceiveDown starts a down-reception assertion on this node.
func (n *Node) ShouldReceiveDown() *DownAssert { return &DownAssert{check.For[Down](n.t, n.rec)} }
func (a *DownAssert) To(consumer gen.PID) *DownAssert {
	a.Where(func(r Down) bool { return r.To == consumer })
	return a
}
func (a *DownAssert) About(target gen.PID) *DownAssert {
	a.Where(func(r Down) bool { m, ok := r.Message.(gen.MessageDownPID); return ok && m.PID == target })
	return a
}
func (a *DownAssert) AboutProcessID(target gen.ProcessID) *DownAssert {
	a.Where(func(r Down) bool { m, ok := r.Message.(gen.MessageDownProcessID); return ok && m.ProcessID == target })
	return a
}
func (a *DownAssert) AboutAlias(target gen.Alias) *DownAssert {
	a.Where(func(r Down) bool { m, ok := r.Message.(gen.MessageDownAlias); return ok && m.Alias == target })
	return a
}
func (a *DownAssert) AboutEvent(target gen.Event) *DownAssert {
	a.Where(func(r Down) bool { m, ok := r.Message.(gen.MessageDownEvent); return ok && m.Event == target })
	return a
}
func (a *DownAssert) Reason(target error) *DownAssert {
	a.Where(func(r Down) bool { reason, ok := downReason(r.Message); return ok && reason == target })
	return a
}

// ReasonIs matches when the down reason wraps target (errors.Is). Use it for a
// cascade termination, where the reason is wrapped (e.g. a non-trapping linked
// process terminating from a partner's exit).
func (a *DownAssert) ReasonIs(target error) *DownAssert {
	a.Where(func(r Down) bool { reason, ok := downReason(r.Message); return ok && errors.Is(reason, target) })
	return a
}

// ExitAssert asserts over exit signals received on a node (ingress).
type ExitAssert struct{ *check.Assertion[Exit] }

// ShouldReceiveExit starts an exit-reception assertion on this node.
func (n *Node) ShouldReceiveExit() *ExitAssert { return &ExitAssert{check.For[Exit](n.t, n.rec)} }
func (a *ExitAssert) To(consumer gen.PID) *ExitAssert {
	a.Where(func(r Exit) bool { return r.To == consumer })
	return a
}
func (a *ExitAssert) About(target gen.PID) *ExitAssert {
	a.Where(func(r Exit) bool { m, ok := r.Message.(gen.MessageExitPID); return ok && m.PID == target })
	return a
}
func (a *ExitAssert) AboutProcessID(target gen.ProcessID) *ExitAssert {
	a.Where(func(r Exit) bool { m, ok := r.Message.(gen.MessageExitProcessID); return ok && m.ProcessID == target })
	return a
}
func (a *ExitAssert) AboutAlias(target gen.Alias) *ExitAssert {
	a.Where(func(r Exit) bool { m, ok := r.Message.(gen.MessageExitAlias); return ok && m.Alias == target })
	return a
}
func (a *ExitAssert) AboutEvent(target gen.Event) *ExitAssert {
	a.Where(func(r Exit) bool { m, ok := r.Message.(gen.MessageExitEvent); return ok && m.Event == target })
	return a
}
func (a *ExitAssert) Reason(target error) *ExitAssert {
	a.Where(func(r Exit) bool { reason, ok := exitReason(r.Message); return ok && reason == target })
	return a
}

// ReasonIs matches when the exit reason wraps target (errors.Is). Use it when the
// reason is wrapped rather than exact, e.g. a process spawned with PreserveMailbox
// terminating abnormally (the reason is captured into a *gen.Error).
func (a *ExitAssert) ReasonIs(target error) *ExitAssert {
	a.Where(func(r Exit) bool { reason, ok := exitReason(r.Message); return ok && errors.Is(reason, target) })
	return a
}

// EventAssert asserts over pub/sub events received on a node (ingress).
type EventAssert struct{ *check.Assertion[Event] }

// ShouldReceiveEvent starts an event-reception assertion on this node.
func (n *Node) ShouldReceiveEvent() *EventAssert { return &EventAssert{check.For[Event](n.t, n.rec)} }
func (a *EventAssert) To(subscriber gen.PID) *EventAssert {
	a.Where(func(r Event) bool { return r.To == subscriber })
	return a
}
func (a *EventAssert) Event(ev gen.Event) *EventAssert {
	a.Where(func(r Event) bool { return r.Event == ev })
	return a
}
func (a *EventAssert) Message(v any) *EventAssert {
	a.Where(func(r Event) bool { return reflect.DeepEqual(r.Message, v) })
	return a
}

// MonitorAssert asserts over monitors set up on a node (egress).
type MonitorAssert struct{ *check.Assertion[Monitored] }

// ShouldMonitor starts a monitor-setup assertion on this node.
func (n *Node) ShouldMonitor() *MonitorAssert {
	return &MonitorAssert{check.For[Monitored](n.t, n.rec)}
}
func (a *MonitorAssert) From(p gen.PID) *MonitorAssert {
	a.Where(func(r Monitored) bool { return r.From == p })
	return a
}
func (a *MonitorAssert) Target(t any) *MonitorAssert {
	a.Where(func(r Monitored) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}
func (a *MonitorAssert) Error(target error) *MonitorAssert {
	a.Where(func(r Monitored) bool { return r.Error == target })
	return a
}

// DemonitorAssert asserts over monitors removed on a node (egress).
type DemonitorAssert struct{ *check.Assertion[Demonitored] }

// ShouldDemonitor starts a demonitor assertion on this node.
func (n *Node) ShouldDemonitor() *DemonitorAssert {
	return &DemonitorAssert{check.For[Demonitored](n.t, n.rec)}
}
func (a *DemonitorAssert) From(p gen.PID) *DemonitorAssert {
	a.Where(func(r Demonitored) bool { return r.From == p })
	return a
}
func (a *DemonitorAssert) Target(t any) *DemonitorAssert {
	a.Where(func(r Demonitored) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}
func (a *DemonitorAssert) Error(target error) *DemonitorAssert {
	a.Where(func(r Demonitored) bool { return r.Error == target })
	return a
}

// LinkAssert asserts over links set up on a node (egress).
type LinkAssert struct{ *check.Assertion[Linked] }

// ShouldLink starts a link-setup assertion on this node.
func (n *Node) ShouldLink() *LinkAssert { return &LinkAssert{check.For[Linked](n.t, n.rec)} }
func (a *LinkAssert) From(p gen.PID) *LinkAssert {
	a.Where(func(r Linked) bool { return r.From == p })
	return a
}
func (a *LinkAssert) Target(t any) *LinkAssert {
	a.Where(func(r Linked) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}
func (a *LinkAssert) Error(target error) *LinkAssert {
	a.Where(func(r Linked) bool { return r.Error == target })
	return a
}

// UnlinkAssert asserts over links removed on a node (egress).
type UnlinkAssert struct{ *check.Assertion[Unlinked] }

// ShouldUnlink starts an unlink assertion on this node.
func (n *Node) ShouldUnlink() *UnlinkAssert { return &UnlinkAssert{check.For[Unlinked](n.t, n.rec)} }
func (a *UnlinkAssert) From(p gen.PID) *UnlinkAssert {
	a.Where(func(r Unlinked) bool { return r.From == p })
	return a
}
func (a *UnlinkAssert) Target(t any) *UnlinkAssert {
	a.Where(func(r Unlinked) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}
func (a *UnlinkAssert) Error(target error) *UnlinkAssert {
	a.Where(func(r Unlinked) bool { return r.Error == target })
	return a
}

// WireLinkAssert asserts over remote links arriving over the wire (ingress).
type WireLinkAssert struct{ *check.Assertion[WireLink] }

// ShouldWireLink starts a wire-link ingress assertion on this node.
func (n *Node) ShouldWireLink() *WireLinkAssert {
	return &WireLinkAssert{check.For[WireLink](n.t, n.rec)}
}
func (a *WireLinkAssert) From(p gen.PID) *WireLinkAssert {
	a.Where(func(r WireLink) bool { return r.From == p })
	return a
}
func (a *WireLinkAssert) Target(t any) *WireLinkAssert {
	a.Where(func(r WireLink) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}

// WireUnlinkAssert asserts over remote unlinks arriving over the wire (ingress).
type WireUnlinkAssert struct{ *check.Assertion[WireUnlink] }

// ShouldWireUnlink starts a wire-unlink ingress assertion on this node.
func (n *Node) ShouldWireUnlink() *WireUnlinkAssert {
	return &WireUnlinkAssert{check.For[WireUnlink](n.t, n.rec)}
}
func (a *WireUnlinkAssert) From(p gen.PID) *WireUnlinkAssert {
	a.Where(func(r WireUnlink) bool { return r.From == p })
	return a
}
func (a *WireUnlinkAssert) Target(t any) *WireUnlinkAssert {
	a.Where(func(r WireUnlink) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}

// WireMonitorAssert asserts over remote monitors arriving over the wire (ingress).
type WireMonitorAssert struct{ *check.Assertion[WireMonitor] }

// ShouldWireMonitor starts a wire-monitor ingress assertion on this node.
func (n *Node) ShouldWireMonitor() *WireMonitorAssert {
	return &WireMonitorAssert{check.For[WireMonitor](n.t, n.rec)}
}
func (a *WireMonitorAssert) From(p gen.PID) *WireMonitorAssert {
	a.Where(func(r WireMonitor) bool { return r.From == p })
	return a
}
func (a *WireMonitorAssert) Target(t any) *WireMonitorAssert {
	a.Where(func(r WireMonitor) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}

// WireDemonitorAssert asserts over remote demonitors arriving over the wire (ingress).
type WireDemonitorAssert struct {
	*check.Assertion[WireDemonitor]
}

// ShouldWireDemonitor starts a wire-demonitor ingress assertion on this node.
func (n *Node) ShouldWireDemonitor() *WireDemonitorAssert {
	return &WireDemonitorAssert{check.For[WireDemonitor](n.t, n.rec)}
}
func (a *WireDemonitorAssert) From(p gen.PID) *WireDemonitorAssert {
	a.Where(func(r WireDemonitor) bool { return r.From == p })
	return a
}
func (a *WireDemonitorAssert) Target(t any) *WireDemonitorAssert {
	a.Where(func(r WireDemonitor) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}

// SendEventAssert asserts over events published on a node (egress).
type SendEventAssert struct{ *check.Assertion[SentEvent] }

// ShouldSendEvent starts an event-publish assertion on this node.
func (n *Node) ShouldSendEvent() *SendEventAssert {
	return &SendEventAssert{check.For[SentEvent](n.t, n.rec)}
}
func (a *SendEventAssert) From(p gen.PID) *SendEventAssert {
	a.Where(func(r SentEvent) bool { return r.From == p })
	return a
}
func (a *SendEventAssert) Name(name gen.Atom) *SendEventAssert {
	a.Where(func(r SentEvent) bool { return r.Name == name })
	return a
}
func (a *SendEventAssert) Error(target error) *SendEventAssert {
	a.Where(func(r SentEvent) bool { return r.Error == target })
	return a
}

// SendResponseAssert asserts over responses a process sent to requests (egress).
type SendResponseAssert struct{ *check.Assertion[SentResponse] }

// ShouldSendResponse starts a response assertion on this node.
func (n *Node) ShouldSendResponse() *SendResponseAssert {
	return &SendResponseAssert{check.For[SentResponse](n.t, n.rec)}
}
func (a *SendResponseAssert) From(p gen.PID) *SendResponseAssert {
	a.Where(func(r SentResponse) bool { return r.From == p })
	return a
}
func (a *SendResponseAssert) To(p gen.PID) *SendResponseAssert {
	a.Where(func(r SentResponse) bool { return r.To == p })
	return a
}
func (a *SendResponseAssert) Message(v any) *SendResponseAssert {
	a.Where(func(r SentResponse) bool { return reflect.DeepEqual(r.Message, v) })
	return a
}
func (a *SendResponseAssert) Error(target error) *SendResponseAssert {
	a.Where(func(r SentResponse) bool { return r.Error == target })
	return a
}
