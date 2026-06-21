// Package stage is the live multi-node system-test harness. It starts real
// nodes, drives them through the public gen.Node API, and observes real
// happenings by decorating each node's routing surface (gen.Core, ingress) and
// each process's gen.Process (egress) via node.NodeOptionsExtra. Assertions use
// the shared testing/check grammar. Unlike testing/unit (mock node, single
// actor), stage runs the actual runtime.
//
// Plane contract: this harness records both ingress (Delivered, Down, Exit, Event,
// and the Wire* subscriptions) and egress. It does NOT produce Terminated (observe a
// process stopping via a Down/Exit on a monitor or link), SendAfter (timers run
// for real; observe the eventual Send/Delivered), or Log (the node logger is
// disabled). Those three are testing/unit-only.
package stage

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ergo.services/ergo/app/system"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/net/handshake"
	"ergo.services/ergo/node"
	"ergo.services/ergo/testing/check"
)

// frameworkVersion is recorded into started nodes. Stage tests do not depend on
// the real framework version, so a fixed stage marker is enough.
var frameworkVersion = gen.Version{Name: "ergo.services/stage", Release: "test"}

var stageSeq atomic.Uint64

// StageOptions configures a stage.
type StageOptions struct {
	// Registrar overrides the default in-memory registrar. When nil, the stage
	// uses a private in-memory registry (no ports, isolated from other stages, so
	// any number of stages run in parallel without contending for a registrar
	// port). It enforces node-name uniqueness the same way the embedded registrar
	// does (a duplicate name fails node start with gen.ErrTaken). Supply a factory
	// (e.g. etcd) for cluster scenarios; it is called once per node, so each node
	// gets its own instance.
	Registrar func() (gen.Registrar, error)

	// RegistrarFull selects the in-memory registrar's feature set (ignored when a
	// custom Registrar factory is set). False (default) is embedded-equivalent: node
	// routes only; ResolveApplication and Event report ErrUnsupported. True adds
	// application-route discovery (ResolveApplication) and the canonical
	// gen.MessageRegistrar* event stream (Registrar().Event()), matching the contract
	// etcd/saturn implement, so applications that use service discovery can run.
	RegistrarFull bool
}

// Stage owns a set of live nodes and tears them down on test cleanup.
type Stage struct {
	t            *testing.T
	id           uint64
	mu           sync.Mutex
	nodes        []*Node
	newRegistrar func() (gen.Registrar, error)
}

// New creates a stage and registers teardown via t.Cleanup. With no options the
// stage uses a private in-memory registrar shared by its nodes.
func New(t *testing.T, opts ...StageOptions) *Stage {
	s := &Stage{t: t, id: stageSeq.Add(1)}
	var o StageOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.Registrar != nil {
		s.newRegistrar = o.Registrar
	} else {
		store := newMemStore(o.RegistrarFull)
		s.newRegistrar = func() (gen.Registrar, error) { return &memRegistrar{store: store}, nil }
	}
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
	// PoolSize sets the number of TCP connections per peer connection. Zero keeps the
	// framework default.
	PoolSize int

	// Security pass-through (e.g. ExposeEnvRemoteSpawn for remote-spawn env inheritance).
	Security gen.SecurityOptions
}

// Node is a live node started by the stage. It embeds *check.Asserter, so the
// whole Should* grammar is available directly on the node.
type Node struct {
	*check.Asserter
	s    *Stage
	t    *testing.T
	node gen.Node
	rec  *check.Recorder
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
	if o.PoolSize > 0 {
		no.Network.Handshake = handshake.Create(handshake.Options{PoolSize: o.PoolSize})
	}
	no.Security = o.Security
	no.Env = o.Env
	if o.EnableSystemApp {
		no.Applications = append([]gen.ApplicationBehavior{system.CreateApp()}, o.Applications...)
	} else {
		no.Applications = o.Applications
	}

	reg, err := s.newRegistrar()
	if err != nil {
		s.t.Fatalf("stage: create registrar for node %s: %s", name, err)
	}
	no.Network.Registrar = reg

	// unique across parallel test processes (pid) and within a process (seq)
	r := check.NewRecorder()
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

	n := &Node{Asserter: check.NewAsserter(s.t, r), s: s, t: s.t, node: gn, rec: r}
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

// Kill force-terminates a process on this node (TerminateReasonKill).
func (n *Node) Kill(pid gen.PID) {
	n.t.Helper()
	if err := n.node.Kill(pid); err != nil {
		n.t.Fatalf("stage: kill %s: %s", pid, err)
	}
}

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

// recordRemoteNode wraps the gen.RemoteNode returned by Connect so node-level remote
// egress is observable: Spawn/SpawnRegister record check.RemoteSpawn and the
// ApplicationStart* family records check.RemoteApplicationStart, attributed to the
// local node (from). Queries and Disconnect delegate to the real remote unchanged.
type recordRemoteNode struct {
	gen.RemoteNode
	from gen.PID
	rec  *check.Recorder
}

func (r *recordRemoteNode) Spawn(name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	pid, err := r.RemoteNode.Spawn(name, options, args...)
	r.rec.Put(check.RemoteSpawn{Parent: r.from, Node: r.RemoteNode.Name(), Name: name, Child: pid, Options: options, Error: err})
	return pid, err
}
func (r *recordRemoteNode) SpawnRegister(register gen.Atom, name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	pid, err := r.RemoteNode.SpawnRegister(register, name, options, args...)
	r.rec.Put(check.RemoteSpawn{Parent: r.from, Node: r.RemoteNode.Name(), Name: name, Register: register, Child: pid, Options: options, Error: err})
	return pid, err
}
func (r *recordRemoteNode) ApplicationStart(name gen.Atom, options gen.ApplicationOptions) error {
	err := r.RemoteNode.ApplicationStart(name, options)
	r.rec.Put(check.RemoteApplicationStart{From: r.from, Node: r.RemoteNode.Name(), Name: name, Error: err})
	return err
}
func (r *recordRemoteNode) ApplicationStartTemporary(name gen.Atom, options gen.ApplicationOptions) error {
	err := r.RemoteNode.ApplicationStartTemporary(name, options)
	r.rec.Put(check.RemoteApplicationStart{From: r.from, Node: r.RemoteNode.Name(), Name: name, Mode: gen.ApplicationModeTemporary, Error: err})
	return err
}
func (r *recordRemoteNode) ApplicationStartTransient(name gen.Atom, options gen.ApplicationOptions) error {
	err := r.RemoteNode.ApplicationStartTransient(name, options)
	r.rec.Put(check.RemoteApplicationStart{From: r.from, Node: r.RemoteNode.Name(), Name: name, Mode: gen.ApplicationModeTransient, Error: err})
	return err
}
func (r *recordRemoteNode) ApplicationStartPermanent(name gen.Atom, options gen.ApplicationOptions) error {
	err := r.RemoteNode.ApplicationStartPermanent(name, options)
	r.rec.Put(check.RemoteApplicationStart{From: r.from, Node: r.RemoteNode.Name(), Name: name, Mode: gen.ApplicationModePermanent, Error: err})
	return err
}

// Connect establishes a connection from a to b and waits until both sides have
// registered it, then returns the RemoteNode from a's perspective (wrapped so its
// Spawn / ApplicationStart* egress is recorded on a's stream). The wait is
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
	return &recordRemoteNode{RemoteNode: remote, from: a.node.PID(), rec: a.rec}
}

// ConnectMesh dials every ordered pair of nodes concurrently, so each link is
// attempted from both sides at once and exercises simultaneous-connect collision
// resolution. It then waits until every node sees every other and every connection
// has filled its TCP pool, re-dialing any missing pair until the mesh settles or
// the deadline passes (a real cluster retries dials that the TCP backlog dropped
// under a connect storm). Duplicate detection is left to the caller via
// Network().Nodes().
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
		unsettled := 0
		for i := range nodes {
			for j := range nodes {
				if i == j {
					continue
				}
				r, err := nodes[i].node.Network().Node(nodes[j].node.Name())
				if err != nil {
					unsettled++
					nodes[i].node.Network().GetNode(nodes[j].node.Name())
					continue
				}
				// settled only once the TCP pool has filled to its target
				if info := r.Info(); info.PoolLen != info.PoolSize {
					unsettled++
				}
			}
		}
		if unsettled == 0 {
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("stage: mesh of %d nodes did not settle: %d pairs missing or pool not filled", len(nodes), unsettled)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// recordCore: ingress on the routing surface (gen.Core)
// Two kinds of happening cross this surface: a message delivered into a local
// mailbox (the egress counterpart is Send), and a remote subscription arriving
// over the wire (the egress counterpart is Link/Monitor).

type recordCore struct {
	gen.Core
	rec *check.Recorder
}

func (c *recordCore) local(node gen.Atom) bool {
	return node == "" || node == c.Core.Name()
}

func (c *recordCore) RouteSendPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	err := c.Core.RouteSendPID(from, to, options, message)
	if err == nil && c.local(to.Node) {
		c.rec.Put(check.Delivered{From: from, To: to, Message: message})
	}
	return err
}

func (c *recordCore) RouteSendProcessID(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error {
	err := c.Core.RouteSendProcessID(from, to, options, message)
	if err == nil && c.local(to.Node) {
		c.rec.Put(check.Delivered{From: from, To: to, Message: message})
	}
	return err
}

func (c *recordCore) RouteCallPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	err := c.Core.RouteCallPID(from, to, options, message)
	if err == nil && c.local(to.Node) {
		c.rec.Put(check.Delivered{From: from, To: to, Message: message})
	}
	return err
}

func (c *recordCore) RouteCallProcessID(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error {
	err := c.Core.RouteCallProcessID(from, to, options, message)
	if err == nil && c.local(to.Node) {
		c.rec.Put(check.Delivered{From: from, To: to, Message: message})
	}
	return err
}

func (c *recordCore) RouteSendAlias(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error {
	err := c.Core.RouteSendAlias(from, to, options, message)
	if err == nil && c.local(to.Node) {
		c.rec.Put(check.Delivered{From: from, To: to, Message: message})
	}
	return err
}

func (c *recordCore) RouteCallAlias(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error {
	err := c.Core.RouteCallAlias(from, to, options, message)
	if err == nil && c.local(to.Node) {
		c.rec.Put(check.Delivered{From: from, To: to, Message: message})
	}
	return err
}

func (c *recordCore) RouteSendExit(from gen.PID, to gen.PID, reason error) error {
	err := c.Core.RouteSendExit(from, to, reason)
	if err == nil && c.local(to.Node) {
		c.rec.Put(check.Exit{To: to, Message: gen.MessageExitPID{PID: from, Reason: reason}})
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
		c.rec.Put(check.WireLink{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteUnlinkPID(pid gen.PID, target gen.PID) error {
	err := c.Core.RouteUnlinkPID(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.Put(check.WireUnlink{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteLinkProcessID(pid gen.PID, target gen.ProcessID) error {
	err := c.Core.RouteLinkProcessID(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.Put(check.WireLink{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteUnlinkProcessID(pid gen.PID, target gen.ProcessID) error {
	err := c.Core.RouteUnlinkProcessID(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.Put(check.WireUnlink{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteLinkAlias(pid gen.PID, target gen.Alias) error {
	err := c.Core.RouteLinkAlias(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.Put(check.WireLink{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteUnlinkAlias(pid gen.PID, target gen.Alias) error {
	err := c.Core.RouteUnlinkAlias(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.Put(check.WireUnlink{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteLinkEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {
	r, err := c.Core.RouteLinkEvent(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.Put(check.WireLink{From: pid, Target: target})
	}
	return r, err
}

func (c *recordCore) RouteUnlinkEvent(pid gen.PID, target gen.Event) error {
	err := c.Core.RouteUnlinkEvent(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.Put(check.WireUnlink{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteMonitorPID(pid gen.PID, target gen.PID) error {
	err := c.Core.RouteMonitorPID(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.Put(check.WireMonitor{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteDemonitorPID(pid gen.PID, target gen.PID) error {
	err := c.Core.RouteDemonitorPID(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.Put(check.WireDemonitor{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteMonitorProcessID(pid gen.PID, target gen.ProcessID) error {
	err := c.Core.RouteMonitorProcessID(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.Put(check.WireMonitor{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteDemonitorProcessID(pid gen.PID, target gen.ProcessID) error {
	err := c.Core.RouteDemonitorProcessID(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.Put(check.WireDemonitor{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteMonitorAlias(pid gen.PID, target gen.Alias) error {
	err := c.Core.RouteMonitorAlias(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.Put(check.WireMonitor{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteDemonitorAlias(pid gen.PID, target gen.Alias) error {
	err := c.Core.RouteDemonitorAlias(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.Put(check.WireDemonitor{From: pid, Target: target})
	}
	return err
}

func (c *recordCore) RouteMonitorEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {
	r, err := c.Core.RouteMonitorEvent(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.Put(check.WireMonitor{From: pid, Target: target})
	}
	return r, err
}

func (c *recordCore) RouteDemonitorEvent(pid gen.PID, target gen.Event) error {
	err := c.Core.RouteDemonitorEvent(pid, target)
	if err == nil && c.local(pid.Node) == false {
		c.rec.Put(check.WireDemonitor{From: pid, Target: target})
	}
	return err
}

// recordBridge: reception of Down/Exit/Event delivered by the target manager
// (these bypass gen.Core; the target manager bridge is their delivery surface)

type recordBridge struct {
	gen.CoreTargetManager
	rec *check.Recorder
}

func (b *recordBridge) RouteSendPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	err := b.CoreTargetManager.RouteSendPID(from, to, options, message)
	if err == nil {
		switch message.(type) {
		case gen.MessageDownPID, gen.MessageDownProcessID, gen.MessageDownAlias, gen.MessageDownNode, gen.MessageDownEvent:
			b.rec.Put(check.Down{To: to, Message: message})
		case gen.MessageEventStart, gen.MessageEventStop:
			// producer notifications about first subscriber / last unsubscribe
			b.rec.Put(check.Delivered{From: from, To: to, Message: message})
		}
	}
	return err
}

func (b *recordBridge) RouteSendExitMessages(from gen.PID, to []gen.PID, message any) error {
	err := b.CoreTargetManager.RouteSendExitMessages(from, to, message)
	if err == nil {
		for _, pid := range to {
			b.rec.Put(check.Exit{To: pid, Message: message})
		}
	}
	return err
}

func (b *recordBridge) RouteSendEventMessages(from gen.PID, to []gen.PID, options gen.MessageOptions, message gen.MessageEvent) error {
	err := b.CoreTargetManager.RouteSendEventMessages(from, to, options, message)
	if err == nil {
		for _, pid := range to {
			b.rec.Put(check.Event{To: pid, Event: message.Event, Timestamp: message.Timestamp, Message: message.Message})
		}
	}
	return err
}

// recordProcess: egress (records a process's outgoing actions)
type recordProcess struct {
	gen.Process
	rec *check.Recorder
}

func (p *recordProcess) Monitor(target any) error {
	err := p.Process.Monitor(target)
	p.rec.Put(check.Monitor{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) MonitorPID(pid gen.PID) error {
	err := p.Process.MonitorPID(pid)
	p.rec.Put(check.Monitor{From: p.Process.PID(), Target: pid, Error: err})
	return err
}

func (p *recordProcess) MonitorProcessID(target gen.ProcessID) error {
	err := p.Process.MonitorProcessID(target)
	p.rec.Put(check.Monitor{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) MonitorAlias(target gen.Alias) error {
	err := p.Process.MonitorAlias(target)
	p.rec.Put(check.Monitor{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) MonitorEvent(target gen.Event) ([]gen.MessageEvent, error) {
	last, err := p.Process.MonitorEvent(target)
	p.rec.Put(check.Monitor{From: p.Process.PID(), Target: target, Error: err})
	return last, err
}

func (p *recordProcess) Demonitor(target any) error {
	err := p.Process.Demonitor(target)
	p.rec.Put(check.Demonitor{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) DemonitorPID(pid gen.PID) error {
	err := p.Process.DemonitorPID(pid)
	p.rec.Put(check.Demonitor{From: p.Process.PID(), Target: pid, Error: err})
	return err
}

func (p *recordProcess) DemonitorProcessID(target gen.ProcessID) error {
	err := p.Process.DemonitorProcessID(target)
	p.rec.Put(check.Demonitor{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) DemonitorAlias(target gen.Alias) error {
	err := p.Process.DemonitorAlias(target)
	p.rec.Put(check.Demonitor{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) DemonitorEvent(target gen.Event) error {
	err := p.Process.DemonitorEvent(target)
	p.rec.Put(check.Demonitor{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) Link(target any) error {
	err := p.Process.Link(target)
	p.rec.Put(check.Link{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) LinkPID(pid gen.PID) error {
	err := p.Process.LinkPID(pid)
	p.rec.Put(check.Link{From: p.Process.PID(), Target: pid, Error: err})
	return err
}

func (p *recordProcess) LinkProcessID(target gen.ProcessID) error {
	err := p.Process.LinkProcessID(target)
	p.rec.Put(check.Link{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) LinkAlias(target gen.Alias) error {
	err := p.Process.LinkAlias(target)
	p.rec.Put(check.Link{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) LinkEvent(target gen.Event) ([]gen.MessageEvent, error) {
	last, err := p.Process.LinkEvent(target)
	p.rec.Put(check.Link{From: p.Process.PID(), Target: target, Error: err})
	return last, err
}

func (p *recordProcess) Unlink(target any) error {
	err := p.Process.Unlink(target)
	p.rec.Put(check.Unlink{From: p.Process.PID(), Target: target, Error: err})
	return err
}
func (p *recordProcess) LinkNode(target gen.Atom) error {
	err := p.Process.LinkNode(target)
	p.rec.Put(check.Link{From: p.Process.PID(), Target: target, Error: err})
	return err
}
func (p *recordProcess) UnlinkNode(target gen.Atom) error {
	err := p.Process.UnlinkNode(target)
	p.rec.Put(check.Unlink{From: p.Process.PID(), Target: target, Error: err})
	return err
}
func (p *recordProcess) MonitorNode(target gen.Atom) error {
	err := p.Process.MonitorNode(target)
	p.rec.Put(check.Monitor{From: p.Process.PID(), Target: target, Error: err})
	return err
}
func (p *recordProcess) DemonitorNode(target gen.Atom) error {
	err := p.Process.DemonitorNode(target)
	p.rec.Put(check.Demonitor{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) UnlinkPID(pid gen.PID) error {
	err := p.Process.UnlinkPID(pid)
	p.rec.Put(check.Unlink{From: p.Process.PID(), Target: pid, Error: err})
	return err
}

func (p *recordProcess) UnlinkProcessID(target gen.ProcessID) error {
	err := p.Process.UnlinkProcessID(target)
	p.rec.Put(check.Unlink{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) UnlinkAlias(target gen.Alias) error {
	err := p.Process.UnlinkAlias(target)
	p.rec.Put(check.Unlink{From: p.Process.PID(), Target: target, Error: err})
	return err
}

func (p *recordProcess) UnlinkEvent(target gen.Event) error {
	err := p.Process.UnlinkEvent(target)
	p.rec.Put(check.Unlink{From: p.Process.PID(), Target: target, Error: err})
	return err
}

// msgOptions reconstructs the effective gen.MessageOptions the wrapped process will
// build for a Send from its current public state - the same fields the real Send
// reads. The egress (Send/SendEvent/SendResponse) is recorded at this process-API
// seam (locality-independent, original target), so it carries the options here
// rather than at the core seam (which sees a resolved target and only local
// deliveries). One-shot overrides (SendWithPriority/SendImportant) are applied from
// the call arguments below, never by mutating process state.
func (p *recordProcess) msgOptions() gen.MessageOptions {
	return gen.MessageOptions{
		Priority: p.Process.SendPriority(),
		Compression: gen.Compression{
			Enable:    p.Process.Compression(),
			Type:      p.Process.CompressionType(),
			Level:     p.Process.CompressionLevel(),
			Threshold: p.Process.CompressionThreshold(),
		},
		KeepNetworkOrder:  p.Process.KeepNetworkOrder(),
		ImportantDelivery: p.Process.ImportantDelivery(),
	}
}

func (p *recordProcess) SendEvent(name gen.Atom, token gen.Ref, message any) error {
	options := p.msgOptions()
	err := p.Process.SendEvent(name, token, message)
	p.rec.Put(check.SendEvent{From: p.Process.PID(), Name: name, Token: token, Message: message, Options: options, Error: err})
	return err
}

func (p *recordProcess) SendResponse(to gen.PID, ref gen.Ref, message any) error {
	options := p.msgOptions()
	err := p.Process.SendResponse(to, ref, message)
	p.rec.Put(check.SendResponse{From: p.Process.PID(), To: to, Ref: ref, Message: message, Options: options, Error: err})
	return err
}

func (p *recordProcess) SendResponseImportant(to gen.PID, ref gen.Ref, message any) error {
	options := p.msgOptions()
	options.ImportantDelivery = true
	err := p.Process.SendResponseImportant(to, ref, message)
	p.rec.Put(check.SendResponse{From: p.Process.PID(), To: to, Ref: ref, Message: message, Options: options, Error: err})
	return err
}

func (p *recordProcess) SendResponseError(to gen.PID, ref gen.Ref, e error) error {
	options := p.msgOptions()
	err := p.Process.SendResponseError(to, ref, e)
	p.rec.Put(check.SendResponse{From: p.Process.PID(), To: to, Ref: ref, Message: e, Options: options, Error: err})
	return err
}

func (p *recordProcess) SendResponseErrorImportant(to gen.PID, ref gen.Ref, e error) error {
	options := p.msgOptions()
	options.ImportantDelivery = true
	err := p.Process.SendResponseErrorImportant(to, ref, e)
	p.rec.Put(check.SendResponse{From: p.Process.PID(), To: to, Ref: ref, Message: e, Options: options, Error: err})
	return err
}

func (p *recordProcess) Send(to any, message any) error {
	options := p.msgOptions()
	err := p.Process.Send(to, message)
	p.rec.Put(check.Send{From: p.Process.PID(), To: to, Message: message, Options: options, Error: err})
	return err
}

func (p *recordProcess) SendPID(to gen.PID, message any) error {
	options := p.msgOptions()
	err := p.Process.SendPID(to, message)
	p.rec.Put(check.Send{From: p.Process.PID(), To: to, Message: message, Options: options, Error: err})
	return err
}

func (p *recordProcess) SendProcessID(to gen.ProcessID, message any) error {
	options := p.msgOptions()
	err := p.Process.SendProcessID(to, message)
	p.rec.Put(check.Send{From: p.Process.PID(), To: to, Message: message, Options: options, Error: err})
	return err
}

func (p *recordProcess) SendAlias(to gen.Alias, message any) error {
	options := p.msgOptions()
	err := p.Process.SendAlias(to, message)
	p.rec.Put(check.Send{From: p.Process.PID(), To: to, Message: message, Options: options, Error: err})
	return err
}

func (p *recordProcess) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	options := p.msgOptions()
	options.Priority = priority
	err := p.Process.SendWithPriority(to, message, priority)
	p.rec.Put(check.Send{From: p.Process.PID(), To: to, Message: message, Options: options, Error: err})
	return err
}

func (p *recordProcess) SendImportant(to any, message any) error {
	options := p.msgOptions()
	options.ImportantDelivery = true
	err := p.Process.SendImportant(to, message)
	p.rec.Put(check.Send{From: p.Process.PID(), To: to, Message: message, Options: options, Error: err})
	return err
}

func (p *recordProcess) SendExit(to gen.PID, reason error) error {
	err := p.Process.SendExit(to, reason)
	p.rec.Put(check.SendExit{From: p.Process.PID(), To: to, Reason: reason, Error: err})
	return err
}
func (p *recordProcess) SendExitMeta(meta gen.Alias, reason error) error {
	err := p.Process.SendExitMeta(meta, reason)
	p.rec.Put(check.SendExitMeta{From: p.Process.PID(), Meta: meta, Reason: reason, Error: err})
	return err
}

func (p *recordProcess) Call(to any, request any) (any, error) {
	response, err := p.Process.Call(to, request)
	p.rec.Put(check.Call{From: p.Process.PID(), To: to, Request: request, Response: response, Error: err})
	return response, err
}

func (p *recordProcess) CallWithTimeout(to any, request any, timeout int) (any, error) {
	response, err := p.Process.CallWithTimeout(to, request, timeout)
	p.rec.Put(check.Call{From: p.Process.PID(), To: to, Request: request, Response: response, Error: err})
	return response, err
}

func (p *recordProcess) CallWithPriority(to any, request any, priority gen.MessagePriority) (any, error) {
	response, err := p.Process.CallWithPriority(to, request, priority)
	p.rec.Put(check.Call{From: p.Process.PID(), To: to, Request: request, Response: response, Error: err})
	return response, err
}

func (p *recordProcess) CallImportant(to any, request any) (any, error) {
	response, err := p.Process.CallImportant(to, request)
	p.rec.Put(check.Call{From: p.Process.PID(), To: to, Request: request, Response: response, Error: err})
	return response, err
}

func (p *recordProcess) CallPID(to gen.PID, request any, timeout int) (any, error) {
	response, err := p.Process.CallPID(to, request, timeout)
	p.rec.Put(check.Call{From: p.Process.PID(), To: to, Request: request, Response: response, Error: err})
	return response, err
}

func (p *recordProcess) CallProcessID(to gen.ProcessID, request any, timeout int) (any, error) {
	response, err := p.Process.CallProcessID(to, request, timeout)
	p.rec.Put(check.Call{From: p.Process.PID(), To: to, Request: request, Response: response, Error: err})
	return response, err
}

func (p *recordProcess) CallAlias(to gen.Alias, request any, timeout int) (any, error) {
	response, err := p.Process.CallAlias(to, request, timeout)
	p.rec.Put(check.Call{From: p.Process.PID(), To: to, Request: request, Response: response, Error: err})
	return response, err
}

func (p *recordProcess) Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	pid, err := p.Process.Spawn(factory, options, args...)
	p.rec.Put(check.Spawn{Parent: p.Process.PID(), Child: pid, Factory: factory, Options: options, Error: err})
	return pid, err
}

func (p *recordProcess) SpawnRegister(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	pid, err := p.Process.SpawnRegister(register, factory, options, args...)
	p.rec.Put(check.Spawn{Parent: p.Process.PID(), Child: pid, Register: register, Factory: factory, Options: options, Error: err})
	return pid, err
}

func (p *recordProcess) RemoteSpawn(node gen.Atom, name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	pid, err := p.Process.RemoteSpawn(node, name, options, args...)
	p.rec.Put(check.RemoteSpawn{Parent: p.Process.PID(), Node: node, Name: name, Child: pid, Options: options, Error: err})
	return pid, err
}

func (p *recordProcess) RemoteSpawnRegister(node gen.Atom, name gen.Atom, register gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	pid, err := p.Process.RemoteSpawnRegister(node, name, register, options, args...)
	p.rec.Put(check.RemoteSpawn{Parent: p.Process.PID(), Node: node, Name: name, Register: register, Child: pid, Options: options, Error: err})
	return pid, err
}

func (p *recordProcess) SpawnMeta(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error) {
	alias, err := p.Process.SpawnMeta(behavior, options)
	p.rec.Put(check.SpawnMeta{Parent: p.Process.PID(), Alias: alias, Error: err})
	return alias, err
}

func (p *recordProcess) CreateAlias() (gen.Alias, error) {
	alias, err := p.Process.CreateAlias()
	p.rec.Put(check.CreateAlias{PID: p.Process.PID(), Alias: alias, Error: err})
	return alias, err
}

func (p *recordProcess) DeleteAlias(alias gen.Alias) error {
	err := p.Process.DeleteAlias(alias)
	p.rec.Put(check.DeleteAlias{PID: p.Process.PID(), Alias: alias, Error: err})
	return err
}

func (p *recordProcess) RegisterEvent(name gen.Atom, options gen.EventOptions) (gen.Ref, error) {
	ref, err := p.Process.RegisterEvent(name, options)
	p.rec.Put(check.RegisterEvent{PID: p.Process.PID(), Name: name, Ref: ref, Error: err})
	return ref, err
}

func (p *recordProcess) UnregisterEvent(name gen.Atom) error {
	err := p.Process.UnregisterEvent(name)
	p.rec.Put(check.UnregisterEvent{PID: p.Process.PID(), Name: name, Error: err})
	return err
}

func (p *recordProcess) Forward(to gen.PID, message *gen.MailboxMessage, priority gen.MessagePriority) error {
	// copy the fields before delegating: once forwarded, the message lives in the
	// target mailbox and may be processed/released concurrently.
	from, msg := message.From, message.Message
	err := p.Process.Forward(to, message, priority)
	p.rec.Put(check.Forward{By: p.Process.PID(), To: to, From: from, Message: msg, Error: err})
	return err
}
