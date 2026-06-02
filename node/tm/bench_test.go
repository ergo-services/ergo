package tm

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

// benchCore: no-op routing + latency-bearing connection. Used to keep
// bench cost dominated by tm internals and by simulated wire RTT, not by
// recording bookkeeping.
type benchCore struct {
	name    gen.Atom
	pid     gen.PID
	latency atomic.Int64 // ns; mutable so setup can prime with zero RTT
}

func newBenchCore(latency time.Duration) *benchCore {
	c := &benchCore{
		name: "node1",
		pid:  gen.PID{Node: "node1", ID: 1, Creation: 100},
	}
	c.latency.Store(int64(latency))
	return c
}

func (c *benchCore) setLatency(d time.Duration) { c.latency.Store(int64(d)) }

func (c *benchCore) Name() gen.Atom { return c.name }
func (c *benchCore) PID() gen.PID   { return c.pid }
func (c *benchCore) Log() gen.Log   { return nil }
func (c *benchCore) MakeRef() gen.Ref {
	return gen.Ref{Node: c.name, Creation: c.pid.Creation}
}
func (c *benchCore) RouteSendPID(gen.PID, gen.PID, gen.MessageOptions, any) error {
	return nil
}
func (c *benchCore) RouteSendExitMessages(gen.PID, []gen.PID, any) error { return nil }
func (c *benchCore) RouteSendEventMessages(gen.PID, []gen.PID, gen.MessageOptions, gen.MessageEvent) error {
	return nil
}
func (c *benchCore) GetConnection(node gen.Atom) (gen.Connection, error) {
	return &benchConn{latency: time.Duration(c.latency.Load())}, nil
}

type benchConn struct {
	latency time.Duration
}

func (c *benchConn) sleep() {
	if c.latency > 0 {
		time.Sleep(c.latency)
	}
}

func (c *benchConn) Node() gen.RemoteNode { return nil }
func (c *benchConn) SendPID(gen.PID, gen.PID, gen.MessageOptions, any) error {
	return nil
}
func (c *benchConn) SendProcessID(gen.PID, gen.ProcessID, gen.MessageOptions, any) error {
	return nil
}
func (c *benchConn) SendAlias(gen.PID, gen.Alias, gen.MessageOptions, any) error {
	return nil
}
func (c *benchConn) SendEvent(gen.PID, gen.MessageOptions, gen.MessageEvent) error {
	return nil
}
func (c *benchConn) SendExit(gen.PID, gen.PID, error) error { return nil }
func (c *benchConn) SendResponse(gen.PID, gen.PID, gen.MessageOptions, any) error {
	return nil
}
func (c *benchConn) SendResponseError(gen.PID, gen.PID, gen.MessageOptions, error) error {
	return nil
}
func (c *benchConn) SendTerminatePID(gen.PID, error) error             { return nil }
func (c *benchConn) SendTerminateProcessID(gen.ProcessID, error) error { return nil }
func (c *benchConn) SendTerminateAlias(gen.Alias, error) error         { return nil }
func (c *benchConn) SendTerminateEvent(gen.Event, error) error         { return nil }
func (c *benchConn) CallPID(gen.PID, gen.PID, gen.MessageOptions, any) error {
	return nil
}
func (c *benchConn) CallProcessID(gen.PID, gen.ProcessID, gen.MessageOptions, any) error {
	return nil
}
func (c *benchConn) CallAlias(gen.PID, gen.Alias, gen.MessageOptions, any) error {
	return nil
}
func (c *benchConn) LinkPID(gen.PID, gen.PID) error  { c.sleep(); return nil }
func (c *benchConn) UnlinkPID(gen.PID, gen.PID) error { c.sleep(); return nil }
func (c *benchConn) LinkProcessID(gen.PID, gen.ProcessID) error  { c.sleep(); return nil }
func (c *benchConn) UnlinkProcessID(gen.PID, gen.ProcessID) error { c.sleep(); return nil }
func (c *benchConn) LinkAlias(gen.PID, gen.Alias) error  { c.sleep(); return nil }
func (c *benchConn) UnlinkAlias(gen.PID, gen.Alias) error { c.sleep(); return nil }
func (c *benchConn) MonitorPID(gen.PID, gen.PID) error  { c.sleep(); return nil }
func (c *benchConn) DemonitorPID(gen.PID, gen.PID) error { c.sleep(); return nil }
func (c *benchConn) MonitorProcessID(gen.PID, gen.ProcessID) error  { c.sleep(); return nil }
func (c *benchConn) DemonitorProcessID(gen.PID, gen.ProcessID) error { c.sleep(); return nil }
func (c *benchConn) MonitorAlias(gen.PID, gen.Alias) error  { c.sleep(); return nil }
func (c *benchConn) DemonitorAlias(gen.PID, gen.Alias) error { c.sleep(); return nil }
func (c *benchConn) LinkEvent(gen.PID, gen.Event) ([]gen.MessageEvent, error) {
	c.sleep()
	return nil, nil
}
func (c *benchConn) UnlinkEvent(gen.PID, gen.Event) error { c.sleep(); return nil }
func (c *benchConn) MonitorEvent(gen.PID, gen.Event) ([]gen.MessageEvent, error) {
	c.sleep()
	return nil, nil
}
func (c *benchConn) DemonitorEvent(gen.PID, gen.Event) error { c.sleep(); return nil }
func (c *benchConn) RemoteSpawn(gen.Atom, gen.ProcessOptionsExtra) (gen.PID, error) {
	return gen.PID{}, nil
}
func (c *benchConn) Join(net.Conn, string, gen.NetworkDial, []byte) error { return nil }
func (c *benchConn) Terminate(error)                                      {}

// Piggyback subscription benchmarks. After the first wire-LinkPID fires
// and establishes wirePresence.state == wirePiggyback, subsequent
// LinkPID calls take the piggyback path under wp.mu.

func BenchmarkPiggyback_OneTarget_10KSubs(b *testing.B) {
	runPiggybackBench(b, 1, 10000, 0)
}

func BenchmarkPiggyback_ManyTargets_10KSubs(b *testing.B) {
	runPiggybackBench(b, 10000, 10000, 0)
}

func BenchmarkPiggyback_OneTarget_1KSubs(b *testing.B) {
	runPiggybackBench(b, 1, 1000, 0)
}

// RTT-bearing variants. The wire-call (first subscriber per target only)
// pays the simulated round-trip; subsequent subscribers piggyback.

func BenchmarkPiggyback_OneTarget_10KSubs_RTT100us(b *testing.B) {
	runPiggybackBench(b, 1, 10000, 100*time.Microsecond)
}
func BenchmarkPiggyback_ManyTargets_10KSubs_RTT100us(b *testing.B) {
	runPiggybackBench(b, 10000, 10000, 100*time.Microsecond)
}
func BenchmarkPiggyback_ManyTargets_10KSubs_RTT1ms(b *testing.B) {
	runPiggybackBench(b, 10000, 10000, 1*time.Millisecond)
}
func BenchmarkPiggyback_ManyTargets_10KSubs_RTT10ms(b *testing.B) {
	runPiggybackBench(b, 10000, 10000, 10*time.Millisecond)
}

func runPiggybackBench(b *testing.B, targets, subscribers int, rtt time.Duration) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Setup runs with zero latency: pre-warming 10K targets at 100us
		// each would dominate wallclock and is not what we are measuring.
		core := newBenchCore(0)
		m := Create(core, Options{}).(*Manager)

		tgts := make([]gen.PID, targets)
		for j := 0; j < targets; j++ {
			tgts[j] = gen.PID{Node: "remote", ID: uint64(j + 100), Creation: 1}
			m.LinkPID(gen.PID{Node: "node1", ID: 1}, tgts[j])
		}

		consumers := make([]gen.PID, subscribers)
		for j := 0; j < subscribers; j++ {
			consumers[j] = gen.PID{Node: "node1", ID: uint64(j + 10000)}
		}
		core.setLatency(rtt)
		b.StartTimer()

		var wg sync.WaitGroup
		wg.Add(subscribers)
		for j := 0; j < subscribers; j++ {
			go func(idx int) {
				defer wg.Done()
				m.LinkPID(consumers[idx], tgts[idx%targets])
			}(j)
		}
		wg.Wait()
	}
}

// First-wire subscription storm: 10K subscribers each to their own remote
// target. Every LinkPID fires a wire-call (no piggyback opportunity).
// Stresses parallel wire-Link path across many independent wirePresence
// slots. This is the user's described 10K-subs / 10K-events scenario.

func BenchmarkFirstWire_1to1_10K_RTT100us(b *testing.B) {
	runFirstWireBench(b, 10000, 100*time.Microsecond)
}
func BenchmarkFirstWire_1to1_10K_RTT1ms(b *testing.B) {
	runFirstWireBench(b, 10000, 1*time.Millisecond)
}
func BenchmarkFirstWire_1to1_10K_RTT10ms(b *testing.B) {
	runFirstWireBench(b, 10000, 10*time.Millisecond)
}

func runFirstWireBench(b *testing.B, n int, rtt time.Duration) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		core := newBenchCore(rtt)
		m := Create(core, Options{}).(*Manager)
		tgts := make([]gen.PID, n)
		consumers := make([]gen.PID, n)
		for j := 0; j < n; j++ {
			tgts[j] = gen.PID{Node: "remote", ID: uint64(j + 100), Creation: 1}
			consumers[j] = gen.PID{Node: "node1", ID: uint64(j + 10000)}
		}
		b.StartTimer()

		var wg sync.WaitGroup
		wg.Add(n)
		for j := 0; j < n; j++ {
			go func(idx int) {
				defer wg.Done()
				m.LinkPID(consumers[idx], tgts[idx])
			}(j)
		}
		wg.Wait()
	}
}

// Race-test variant: half goroutines link, half unlink, on a small set
// of targets. Stresses the (target, kind) wirePresence mu under mixed
// add/remove load.
func BenchmarkLinkUnlink_8Targets_10KOps(b *testing.B) {
	const targets = 8
	const ops = 10000
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		core := newBenchCore(0)
		m := Create(core, Options{}).(*Manager)

		tgts := make([]gen.PID, targets)
		for j := 0; j < targets; j++ {
			tgts[j] = gen.PID{Node: "remote", ID: uint64(j + 100), Creation: 1}
			m.LinkPID(gen.PID{Node: "node1", ID: 1}, tgts[j])
		}

		consumers := make([]gen.PID, ops)
		for j := 0; j < ops; j++ {
			consumers[j] = gen.PID{Node: "node1", ID: uint64(j + 10000)}
			m.LinkPID(consumers[j], tgts[j%targets])
		}
		b.StartTimer()

		var wg sync.WaitGroup
		wg.Add(ops)
		for j := 0; j < ops; j++ {
			go func(idx int) {
				defer wg.Done()
				if idx%2 == 0 {
					m.UnlinkPID(consumers[idx], tgts[idx%targets])
					return
				}
				m.LinkPID(gen.PID{Node: "node1", ID: uint64(idx + 30000)}, tgts[idx%targets])
			}(j)
		}
		wg.Wait()
	}
}
