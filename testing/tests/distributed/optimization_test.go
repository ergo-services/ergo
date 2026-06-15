package distributed

import (
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// subscribeRemote spawns a subscriber on n and links/monitors target on the peer
// (synchronously, so the wire subscription has been acked when it returns). A link
// subscriber traps so it observes the exit as a message.
func subscribeRemote(t *testing.T, n *stage.Node, kind string, target any) gen.PID {
	t.Helper()
	if kind == "link" {
		sub := n.Spawn(factoryRLinker, gen.ProcessOptions{}, true)
		res, err := n.Call(sub, linkTarget{Target: target})
		check.NoError(t, err)
		check.Equal(t, "", res)
		return sub
	}
	sub := n.Spawn(factoryRMonitor, gen.ProcessOptions{})
	res, err := n.Call(sub, monitorTarget{Target: target})
	check.NoError(t, err)
	check.Equal(t, "", res)
	return sub
}

func unsubscribeRemote(t *testing.T, n *stage.Node, kind string, sub gen.PID, target any) {
	t.Helper()
	var res any
	var err error
	if kind == "link" {
		res, err = n.Call(sub, unlinkTarget{Target: target})
	} else {
		res, err = n.Call(sub, demonitorTarget{Target: target})
	}
	check.NoError(t, err)
	check.Equal(t, "", res)
}

// expectRemoteSignal asserts the subscriber received the death notification (exit
// for a link, down for a monitor) carrying the target identity.
func expectRemoteSignal(t *testing.T, n *stage.Node, kind string, sub gen.PID, target any, reason error, mk int) {
	t.Helper()
	if kind == "link" {
		exitAbout(n.ShouldReceiveExit().To(sub), target).Reason(reason).
			Since(mk).Once().Within(time.Second).Must()
		return
	}
	downAbout(n.ShouldReceiveDown().To(sub), target).ReasonIs(reason).
		Since(mk).Once().Within(time.Second).Must()
}

// expectNoRemoteSignal asserts (as a post-barrier snapshot) that the subscriber got
// no death notification. Call only after a positive barrier on a still-subscribed
// peer, so the death fan-out is already recorded.
func expectNoRemoteSignal(t *testing.T, n *stage.Node, kind string, sub gen.PID, mk int) {
	t.Helper()
	if kind == "link" {
		n.ShouldReceiveExit().To(sub).Since(mk).None().Assert()
		return
	}
	n.ShouldReceiveDown().To(sub).Since(mk).None().Assert()
}

// shouldWireSub / shouldWireUnsub select the wire assertion for the kind.
func shouldWireSub(n *stage.Node, kind string, target any) {
	if kind == "link" {
		n.ShouldWireLink().Target(target).Once().Within(time.Second).Must()
		return
	}
	n.ShouldWireMonitor().Target(target).Once().Within(time.Second).Must()
}

// TestDistOptFanOut: multiple subscribers on one node to the same remote target
// share a single wire subscription (the sender deduplicates), yet every local
// subscriber receives the death notification when the target terminates. Covers
// link and monitor across all addressing modes.
func TestDistOptFanOut(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")
	s.Connect(n1, n2)

	for _, kind := range []string{"link", "monitor"} {
		for _, addr := range []string{"pid", "processid", "alias", "event"} {
			t.Run(kind+"/"+addr, func(t *testing.T) {
				target, pid := newRTarget(t, n2, addr)

				subs := []gen.PID{
					subscribeRemote(t, n1, kind, target),
					subscribeRemote(t, n1, kind, target),
					subscribeRemote(t, n1, kind, target),
				}

				// three local subscribers, exactly one subscription on the wire
				shouldWireSub(n2, kind, target)

				mk := n1.Mark()
				check.NoError(t, n2.SendExit(pid, gen.TerminateReasonShutdown))
				for _, sub := range subs {
					expectRemoteSignal(t, n1, kind, sub, target, gen.TerminateReasonShutdown, mk)
				}
			})
		}
	}
}

// TestDistOptWireUnsubOnLast: the wire subscription is torn down only when the last
// local subscriber leaves; earlier unsubscribes are local-only.
func TestDistOptWireUnsubOnLast(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")
	s.Connect(n1, n2)

	for _, kind := range []string{"link", "monitor"} {
		t.Run(kind, func(t *testing.T) {
			target, _ := newRTarget(t, n2, "pid")
			subs := []gen.PID{
				subscribeRemote(t, n1, kind, target),
				subscribeRemote(t, n1, kind, target),
				subscribeRemote(t, n1, kind, target),
			}
			shouldWireSub(n2, kind, target)

			// removing all but the last sends nothing on the wire
			unsubscribeRemote(t, n1, kind, subs[0], target)
			unsubscribeRemote(t, n1, kind, subs[1], target)
			if kind == "link" {
				n2.ShouldWireUnlink().Target(target).None().Assert()
			} else {
				n2.ShouldWireDemonitor().Target(target).None().Assert()
			}

			// removing the last one tears the wire subscription down
			unsubscribeRemote(t, n1, kind, subs[2], target)
			if kind == "link" {
				n2.ShouldWireUnlink().Target(target).Once().Within(time.Second).Must()
			} else {
				n2.ShouldWireDemonitor().Target(target).Once().Within(time.Second).Must()
			}
		})
	}
}

// TestDistOptPartialThenDeath: after some subscribers leave, the survivors still
// receive the death notification and the departed ones receive nothing.
func TestDistOptPartialThenDeath(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")
	s.Connect(n1, n2)

	for _, kind := range []string{"link", "monitor"} {
		t.Run(kind, func(t *testing.T) {
			target, pid := newRTarget(t, n2, "pid")
			gone1 := subscribeRemote(t, n1, kind, target)
			gone2 := subscribeRemote(t, n1, kind, target)
			alive := subscribeRemote(t, n1, kind, target)

			unsubscribeRemote(t, n1, kind, gone1, target)
			unsubscribeRemote(t, n1, kind, gone2, target)

			mk := n1.Mark()
			check.NoError(t, n2.SendExit(pid, gen.TerminateReasonShutdown))
			// survivor receives (positive barrier)
			expectRemoteSignal(t, n1, kind, alive, target, gen.TerminateReasonShutdown, mk)
			// the departed received nothing (snapshot after the barrier)
			expectNoRemoteSignal(t, n1, kind, gone1, mk)
			expectNoRemoteSignal(t, n1, kind, gone2, mk)
		})
	}
}

// TestDistOptDuplicate: a process linking the same remote target twice is rejected
// with ErrTargetExist (the local registration already holds it).
func TestDistOptDuplicate(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")
	s.Connect(n1, n2)

	target, _ := newRTarget(t, n2, "pid")
	sub := n1.Spawn(factoryRLinker, gen.ProcessOptions{}, true)

	res, err := n1.Call(sub, linkTarget{Target: target})
	check.NoError(t, err)
	check.Equal(t, "", res)

	res2, err := n1.Call(sub, linkTarget{Target: target})
	check.NoError(t, err)
	check.Equal(t, gen.ErrTargetExist.Error(), res2)
}

// TestDistOptMixed: a single process both links and monitors the same remote
// target; the two subscriptions are independent on the wire and both fire on death.
func TestDistOptMixed(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")
	s.Connect(n1, n2)

	target, pid := newRTarget(t, n2, "pid")
	sub := n1.Spawn(factoryRLinker, gen.ProcessOptions{}, true)

	res, err := n1.Call(sub, linkTarget{Target: target})
	check.NoError(t, err)
	check.Equal(t, "", res)
	res, err = n1.Call(sub, monitorTarget{Target: target})
	check.NoError(t, err)
	check.Equal(t, "", res)

	// link and monitor are distinct subscriptions on the wire
	n2.ShouldWireLink().Target(target).Once().Within(time.Second).Must()
	n2.ShouldWireMonitor().Target(target).Once().Within(time.Second).Must()

	mk := n1.Mark()
	check.NoError(t, n2.SendExit(pid, gen.TerminateReasonShutdown))
	n1.ShouldReceiveExit().To(sub).About(target.(gen.PID)).Reason(gen.TerminateReasonShutdown).
		Since(mk).Once().Within(time.Second).Must()
	n1.ShouldReceiveDown().To(sub).About(target.(gen.PID)).ReasonIs(gen.TerminateReasonShutdown).
		Since(mk).Once().Within(time.Second).Must()
}

// TestDistOptCrossNode: subscribers on two different nodes each open their own wire
// subscription to the target's node (dedup is per node), and all fire on death.
func TestDistOptCrossNode(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")
	n3 := s.Node("n3")
	s.Connect(n1, n2)
	s.Connect(n3, n2)

	target, pid := newRTarget(t, n2, "pid")
	sub1 := subscribeRemote(t, n1, "link", target)
	sub1b := subscribeRemote(t, n1, "link", target)
	sub3 := subscribeRemote(t, n3, "link", target)

	// one wire link per subscribing node (n1 deduped its two subscribers)
	n2.ShouldWireLink().Target(target).From(n1.PID()).Once().Within(time.Second).Must()
	n2.ShouldWireLink().Target(target).From(n3.PID()).Once().Within(time.Second).Must()

	mk1 := n1.Mark()
	mk3 := n3.Mark()
	check.NoError(t, n2.SendExit(pid, gen.TerminateReasonShutdown))
	for _, sub := range []gen.PID{sub1, sub1b} {
		n1.ShouldReceiveExit().To(sub).About(target.(gen.PID)).Reason(gen.TerminateReasonShutdown).
			Since(mk1).Once().Within(time.Second).Must()
	}
	n3.ShouldReceiveExit().To(sub3).About(target.(gen.PID)).Reason(gen.TerminateReasonShutdown).
		Since(mk3).Once().Within(time.Second).Must()
}

// TestDistOptEventDelivery: every consumer of a remote event receives the buffered
// snapshot on subscribe and the live event on publish.
func TestDistOptEventDelivery(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")
	s.Connect(n1, n2)

	prod := n2.Spawn(factoryEventProducer, gen.ProcessOptions{}, false)
	evAny, err := n2.Call(prod, "create")
	check.NoError(t, err)
	ev := evAny.(gen.Event)

	var cons []gen.PID
	for i := 0; i < 3; i++ {
		c := n1.Spawn(factoryEventConsumer, gen.ProcessOptions{})
		buf, err := n1.Call(c, linkEv{Event: ev})
		check.NoError(t, err)
		check.Equal(t, "buffered", firstMessage(buf))
		cons = append(cons, c)
	}

	mk := n1.Mark()
	_, err = n2.Call(prod, "send")
	check.NoError(t, err)
	for _, c := range cons {
		n1.ShouldReceiveEvent().To(c).Message("live").Since(mk).Once().Within(time.Second).Must()
	}
}

// TestDistOptBufferedEventNoDedup: a buffered event is the exception to wire
// deduplication. Each subscriber makes its own wire subscription so it receives a
// fresh buffer snapshot, so three subscribers produce three wire links (contrast
// with TestDistOptFanOut link/event, where a non-buffered event deduplicates to one).
func TestDistOptBufferedEventNoDedup(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")
	s.Connect(n1, n2)

	prod := n2.Spawn(factoryEventProducer, gen.ProcessOptions{}, false)
	evAny, err := n2.Call(prod, "create")
	check.NoError(t, err)
	ev := evAny.(gen.Event)

	for i := 0; i < 3; i++ {
		c := n1.Spawn(factoryEventConsumer, gen.ProcessOptions{})
		buf, err := n1.Call(c, linkEv{Event: ev})
		check.NoError(t, err)
		check.Equal(t, "buffered", firstMessage(buf))
	}
	n2.ShouldWireLink().Target(ev).From(n1.PID()).Times(3).Within(time.Second).Must()
}

// TestDistOptSubscriberTermination: when a subscriber process terminates its
// subscription is cleaned up like an explicit unlink. The wire subscription
// survives a non-last subscriber's death and is torn down exactly once, by
// whichever death removes the last local subscriber.
func TestDistOptSubscriberTermination(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")
	s.Connect(n1, n2)

	target, _ := newRTarget(t, n2, "pid")
	w := n1.Spawn(factoryWatcher, gen.ProcessOptions{})
	sub1 := subscribeRemote(t, n1, "link", target)
	sub2 := subscribeRemote(t, n1, "link", target)
	n2.ShouldWireLink().Target(target).Once().Within(time.Second).Must()

	// kill a non-last subscriber (sequenced via its Down)
	n1.Send(w, monitorCmd{Target: sub1})
	n1.ShouldMonitor().From(w).Target(sub1).Once().Within(time.Second).Must()
	mk := n1.Mark()
	n1.Kill(sub1)
	n1.ShouldReceiveDown().To(w).About(sub1).Since(mk).Once().Within(time.Second).Must()

	// kill the last subscriber
	n1.Send(w, monitorCmd{Target: sub2})
	n1.ShouldMonitor().From(w).Target(sub2).Once().Within(time.Second).Must()
	mk2 := n1.Mark()
	n1.Kill(sub2)
	n1.ShouldReceiveDown().To(w).About(sub2).Since(mk2).Once().Within(time.Second).Must()

	// exactly one wire unlink total: the non-last death sent nothing on the wire
	n2.ShouldWireUnlink().Target(target).Once().Within(time.Second).Must()
}

// subEvent / unsubEvent drive an event consumer by link or monitor.
func subEvent(t *testing.T, n *stage.Node, kind string, ev gen.Event) gen.PID {
	t.Helper()
	c := n.Spawn(factoryEventConsumer, gen.ProcessOptions{})
	var err error
	if kind == "link" {
		_, err = n.Call(c, linkEv{Event: ev})
	} else {
		_, err = n.Call(c, monitorEv{Event: ev})
	}
	check.NoError(t, err)
	return c
}

func unsubEvent(t *testing.T, n *stage.Node, kind string, c gen.PID) {
	t.Helper()
	var err error
	if kind == "link" {
		_, err = n.Call(c, unlinkEv{})
	} else {
		_, err = n.Call(c, unmonitorEv{})
	}
	check.NoError(t, err)
}

// TestDistOptNotify: a producer with Notify is told exactly once when its first
// subscriber arrives (MessageEventStart) and exactly once when its last subscriber
// leaves (MessageEventStop), regardless of how many subscribers there are or where
// they live. With Notify off it is never told.
func TestDistOptNotify(t *testing.T) {
	// helpers asserting on the producer's node recorder
	start := func(np *stage.Node, prod gen.PID, ev gen.Event, mk int) {
		np.ShouldDeliver().To(prod).Message(gen.MessageEventStart{Name: ev.Name}).
			Since(mk).Once().Within(time.Second).Must()
	}
	noStart := func(np *stage.Node, prod gen.PID, ev gen.Event, mk int) {
		np.ShouldDeliver().To(prod).Message(gen.MessageEventStart{Name: ev.Name}).Since(mk).None().Assert()
	}
	stop := func(np *stage.Node, prod gen.PID, ev gen.Event, mk int) {
		np.ShouldDeliver().To(prod).Message(gen.MessageEventStop{Name: ev.Name}).
			Since(mk).Once().Within(time.Second).Must()
	}
	noStop := func(np *stage.Node, prod gen.PID, ev gen.Event, mk int) {
		np.ShouldDeliver().To(prod).Message(gen.MessageEventStop{Name: ev.Name}).Since(mk).None().Assert()
	}

	// local: producer and both subscribers on one node
	t.Run("Local", func(t *testing.T) {
		s := stage.New(t)
		n := s.Node("n")
		prod := n.Spawn(factoryEventProducer, gen.ProcessOptions{}, true)
		evAny, err := n.Call(prod, "create")
		check.NoError(t, err)
		ev := evAny.(gen.Event)

		mk := n.Mark()
		c1 := subEvent(t, n, "link", ev)
		start(n, prod, ev, mk)

		mk = n.Mark()
		c2 := subEvent(t, n, "link", ev)
		noStart(n, prod, ev, mk)

		mk = n.Mark()
		unsubEvent(t, n, "link", c1)
		noStop(n, prod, ev, mk)

		mk = n.Mark()
		unsubEvent(t, n, "link", c2)
		stop(n, prod, ev, mk)
	})

	// remote: producer on n2, subscribers on n1
	t.Run("Remote", func(t *testing.T) {
		s := stage.New(t)
		n1 := s.Node("n1")
		n2 := s.Node("n2")
		s.Connect(n1, n2)
		prod := n2.Spawn(factoryEventProducer, gen.ProcessOptions{}, true)
		evAny, err := n2.Call(prod, "create")
		check.NoError(t, err)
		ev := evAny.(gen.Event)

		mk := n2.Mark()
		c1 := subEvent(t, n1, "link", ev)
		start(n2, prod, ev, mk)

		mk = n2.Mark()
		c2 := subEvent(t, n1, "link", ev)
		noStart(n2, prod, ev, mk)

		mk = n2.Mark()
		unsubEvent(t, n1, "link", c1)
		noStop(n2, prod, ev, mk)

		mk = n2.Mark()
		unsubEvent(t, n1, "link", c2)
		stop(n2, prod, ev, mk)
	})

	// multi-node: producer on n1, one subscriber each on n2 and n3
	t.Run("MultiNode", func(t *testing.T) {
		s := stage.New(t)
		n1 := s.Node("n1")
		n2 := s.Node("n2")
		n3 := s.Node("n3")
		s.Connect(n2, n1)
		s.Connect(n3, n1)
		prod := n1.Spawn(factoryEventProducer, gen.ProcessOptions{}, true)
		evAny, err := n1.Call(prod, "create")
		check.NoError(t, err)
		ev := evAny.(gen.Event)

		mk := n1.Mark()
		c2 := subEvent(t, n2, "link", ev)
		start(n1, prod, ev, mk)

		mk = n1.Mark()
		c3 := subEvent(t, n3, "link", ev)
		noStart(n1, prod, ev, mk)

		mk = n1.Mark()
		unsubEvent(t, n2, "link", c2)
		noStop(n1, prod, ev, mk)

		mk = n1.Mark()
		unsubEvent(t, n3, "link", c3)
		stop(n1, prod, ev, mk)
	})

	// link and monitor consumers mixed on one node
	t.Run("LinkMonitorMix", func(t *testing.T) {
		s := stage.New(t)
		n := s.Node("n")
		prod := n.Spawn(factoryEventProducer, gen.ProcessOptions{}, true)
		evAny, err := n.Call(prod, "create")
		check.NoError(t, err)
		ev := evAny.(gen.Event)

		mk := n.Mark()
		linkC := subEvent(t, n, "link", ev)
		start(n, prod, ev, mk)

		mk = n.Mark()
		monC := subEvent(t, n, "monitor", ev)
		noStart(n, prod, ev, mk)

		mk = n.Mark()
		unsubEvent(t, n, "link", linkC)
		noStop(n, prod, ev, mk)

		mk = n.Mark()
		unsubEvent(t, n, "monitor", monC)
		stop(n, prod, ev, mk)
	})

	// mixed: producer on n1 with one local subscriber and one remote on n2
	t.Run("Mixed", func(t *testing.T) {
		s := stage.New(t)
		n1 := s.Node("n1")
		n2 := s.Node("n2")
		s.Connect(n2, n1)
		prod := n1.Spawn(factoryEventProducer, gen.ProcessOptions{}, true)
		evAny, err := n1.Call(prod, "create")
		check.NoError(t, err)
		ev := evAny.(gen.Event)

		mk := n1.Mark()
		localC := subEvent(t, n1, "link", ev)
		start(n1, prod, ev, mk)

		mk = n1.Mark()
		remoteC := subEvent(t, n2, "link", ev)
		noStart(n1, prod, ev, mk)

		mk = n1.Mark()
		unsubEvent(t, n1, "link", localC)
		noStop(n1, prod, ev, mk)

		mk = n1.Mark()
		unsubEvent(t, n2, "link", remoteC)
		stop(n1, prod, ev, mk)
	})

	// notify disabled: producer is never told
	t.Run("NoNotify", func(t *testing.T) {
		s := stage.New(t)
		n := s.Node("n")
		prod := n.Spawn(factoryEventProducer, gen.ProcessOptions{}, false)
		evAny, err := n.Call(prod, "create")
		check.NoError(t, err)
		ev := evAny.(gen.Event)

		mk := n.Mark()
		c := subEvent(t, n, "link", ev)
		noStart(n, prod, ev, mk)

		mk = n.Mark()
		unsubEvent(t, n, "link", c)
		noStop(n, prod, ev, mk)
	})
}
