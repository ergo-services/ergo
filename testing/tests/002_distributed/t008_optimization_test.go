package distributed

import (
	"fmt"
	"testing"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

// Test 1.1 from OPTIMIZATION_TEST_PLAN:
// Multiple processes on n1 link to same PID on n2
// Verify all local processes registered
// When target terminates, all receive exit and terminate

func TestT8MultipleLinksToSamePID(t *testing.T) {
	// Start two nodes
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	// Connect nodes
	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn target on node2
	targetPID, err := node2.Spawn(factory_t8target, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Spawn 3 subscribers on node1 - all will link to SAME target
	sub1, err := node1.Spawn(factory_t8linkSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8linkSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8linkSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for all to link
	time.Sleep(200 * time.Millisecond)

	// Verify all are linked
	info1, _ := node1.ProcessInfo(sub1)
	if len(info1.LinksPID) != 1 || info1.LinksPID[0] != targetPID {
		t.Fatalf("sub1 not linked: %v", info1.LinksPID)
	}

	info2, _ := node1.ProcessInfo(sub2)
	if len(info2.LinksPID) != 1 || info2.LinksPID[0] != targetPID {
		t.Fatalf("sub2 not linked: %v", info2.LinksPID)
	}

	info3, _ := node1.ProcessInfo(sub3)
	if len(info3.LinksPID) != 1 || info3.LinksPID[0] != targetPID {
		t.Fatalf("sub3 not linked: %v", info3.LinksPID)
	}

	// Terminate target
	node2.SendExit(targetPID, fmt.Errorf("test"))

	// Wait for propagation
	time.Sleep(300 * time.Millisecond)

	// Verify all terminated
	if _, err := node1.ProcessInfo(sub1); err != gen.ErrProcessUnknown {
		t.Error("sub1 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub2); err != gen.ErrProcessUnknown {
		t.Error("sub2 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub3); err != gen.ErrProcessUnknown {
		t.Error("sub3 should be terminated")
	}
}

func factory_t8target() gen.ProcessBehavior {
	return &t8target{}
}

type t8target struct {
	act.Actor
}

func factory_t8linkSub() gen.ProcessBehavior {
	return &t8linkSub{}
}

type t8linkSub struct {
	act.Actor
	target gen.PID
}

func (s *t8linkSub) Init(args ...any) error {
	s.target = args[0].(gen.PID)
	s.SetTrapExit(true)

	// Can't link in Init - send message to self to do it after
	s.Send(s.PID(), "dolink")
	return nil
}

func (s *t8linkSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "dolink" {
			return s.LinkPID(s.target)
		}
	case gen.MessageExitPID:
		// Target terminated - we terminate too
		return gen.TerminateReasonNormal
	}
	return nil
}

func (s *t8linkSub) Terminate(reason error) {
	s.Log().Warning("terminated: %v", reason)
}

// Test 1.2: Multiple monitors to same PID
func TestT8MultipleMonitorsToSamePID(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1m@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2m@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn target on node2
	targetPID, err := node2.Spawn(factory_t8target, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Spawn 3 subscribers that will monitor same target
	sub1, err := node1.Spawn(factory_t8monitorSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8monitorSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8monitorSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify all are monitoring
	info1, _ := node1.ProcessInfo(sub1)
	if len(info1.MonitorsPID) != 1 || info1.MonitorsPID[0] != targetPID {
		t.Fatalf("sub1 not monitoring: %v", info1.MonitorsPID)
	}

	info2, _ := node1.ProcessInfo(sub2)
	if len(info2.MonitorsPID) != 1 || info2.MonitorsPID[0] != targetPID {
		t.Fatalf("sub2 not monitoring: %v", info2.MonitorsPID)
	}

	info3, _ := node1.ProcessInfo(sub3)
	if len(info3.MonitorsPID) != 1 || info3.MonitorsPID[0] != targetPID {
		t.Fatalf("sub3 not monitoring: %v", info3.MonitorsPID)
	}

	// Terminate target
	node2.SendExit(targetPID, fmt.Errorf("test"))

	time.Sleep(300 * time.Millisecond)

	// Verify all terminated
	if _, err := node1.ProcessInfo(sub1); err != gen.ErrProcessUnknown {
		t.Error("sub1 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub2); err != gen.ErrProcessUnknown {
		t.Error("sub2 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub3); err != gen.ErrProcessUnknown {
		t.Error("sub3 should be terminated")
	}
}

func factory_t8monitorSub() gen.ProcessBehavior {
	return &t8monitorSub{}
}

type t8monitorSub struct {
	act.Actor
	target gen.PID
}

func (s *t8monitorSub) Init(args ...any) error {
	s.target = args[0].(gen.PID)

	// Can't monitor in Init
	s.Send(s.PID(), "domonitor")
	return nil
}

func (s *t8monitorSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "domonitor" {
			return s.MonitorPID(s.target)
		}
	case gen.MessageDownPID:
		// Target down - we terminate
		return gen.TerminateReasonNormal
	}
	return nil
}

// Test 2.1: Partial unlink
// P1, P2, P3 link to target
// P1 unlinks -> P2, P3 still linked, NO remote unlink
// P2 unlinks -> P3 still linked, NO remote unlink
// P3 unlinks -> remote unlink sent (last one)
func TestT8PartialUnlink(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1u@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2u@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn target on node2
	targetPID, err := node2.Spawn(factory_t8target, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Spawn 3 subscribers that can unlink on command
	sub1, err := node1.Spawn(factory_t8unlinkSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8unlinkSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8unlinkSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify all linked
	info1, _ := node1.ProcessInfo(sub1)
	if len(info1.LinksPID) != 1 {
		t.Fatal("sub1 should be linked")
	}

	// P1 unlinks
	node1.Send(sub1, "unlink")
	time.Sleep(100 * time.Millisecond)

	// P1 no longer linked
	info1, _ = node1.ProcessInfo(sub1)
	if len(info1.LinksPID) != 0 {
		t.Error("sub1 should not be linked after unlink")
	}

	// P2, P3 still alive and linked
	if _, err := node1.ProcessInfo(sub2); err != nil {
		t.Error("sub2 should still be alive")
	}
	if _, err := node1.ProcessInfo(sub3); err != nil {
		t.Error("sub3 should still be alive")
	}

	// P2 unlinks
	node1.Send(sub2, "unlink")
	time.Sleep(100 * time.Millisecond)

	// P3 still alive and linked
	if _, err := node1.ProcessInfo(sub3); err != nil {
		t.Error("sub3 should still be alive")
	}

	// P3 unlinks (last one)
	node1.Send(sub3, "unlink")
	time.Sleep(100 * time.Millisecond)

	// Now terminate target - nobody should get exit
	node2.SendExit(targetPID, fmt.Errorf("test"))
	time.Sleep(200 * time.Millisecond)

	// All subs still alive (nobody linked anymore)
	if _, err := node1.ProcessInfo(sub1); err != nil {
		t.Error("sub1 should still be alive (not linked)")
	}
	if _, err := node1.ProcessInfo(sub2); err != nil {
		t.Error("sub2 should still be alive (not linked)")
	}
	if _, err := node1.ProcessInfo(sub3); err != nil {
		t.Error("sub3 should still be alive (not linked)")
	}
}

func factory_t8unlinkSub() gen.ProcessBehavior {
	return &t8unlinkSub{}
}

type t8unlinkSub struct {
	act.Actor
	target gen.PID
}

func (s *t8unlinkSub) Init(args ...any) error {
	s.target = args[0].(gen.PID)
	s.SetTrapExit(true)
	s.Send(s.PID(), "dolink")
	return nil
}

func (s *t8unlinkSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "dolink" {
			return s.LinkPID(s.target)
		}
		if m == "unlink" {
			return s.UnlinkPID(s.target)
		}
	case gen.MessageExitPID:
		return gen.TerminateReasonNormal
	}
	return nil
}

// Test 2.2: Partial demonitor
func TestT8PartialDemonitor(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1d@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2d@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	targetPID, err := node2.Spawn(factory_t8target, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	sub1, err := node1.Spawn(factory_t8demonitorSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8demonitorSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8demonitorSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify all monitoring
	info1, _ := node1.ProcessInfo(sub1)
	if len(info1.MonitorsPID) != 1 {
		t.Fatal("sub1 should be monitoring")
	}

	// P1 demonitors
	node1.Send(sub1, "demonitor")
	time.Sleep(100 * time.Millisecond)

	info1, _ = node1.ProcessInfo(sub1)
	if len(info1.MonitorsPID) != 0 {
		t.Error("sub1 should not be monitoring after demonitor")
	}

	// P2, P3 still alive
	if _, err := node1.ProcessInfo(sub2); err != nil {
		t.Error("sub2 should be alive")
	}
	if _, err := node1.ProcessInfo(sub3); err != nil {
		t.Error("sub3 should be alive")
	}

	// P2 demonitors
	node1.Send(sub2, "demonitor")
	time.Sleep(100 * time.Millisecond)

	// P3 still alive
	if _, err := node1.ProcessInfo(sub3); err != nil {
		t.Error("sub3 should be alive")
	}

	// P3 demonitors (last)
	node1.Send(sub3, "demonitor")
	time.Sleep(100 * time.Millisecond)

	// Terminate target - nobody should get down
	node2.SendExit(targetPID, fmt.Errorf("test"))
	time.Sleep(200 * time.Millisecond)

	// All still alive (nobody monitoring)
	if _, err := node1.ProcessInfo(sub1); err != nil {
		t.Error("sub1 should be alive")
	}
	if _, err := node1.ProcessInfo(sub2); err != nil {
		t.Error("sub2 should be alive")
	}
	if _, err := node1.ProcessInfo(sub3); err != nil {
		t.Error("sub3 should be alive")
	}
}

func factory_t8demonitorSub() gen.ProcessBehavior {
	return &t8demonitorSub{}
}

type t8demonitorSub struct {
	act.Actor
	target gen.PID
}

func (s *t8demonitorSub) Init(args ...any) error {
	s.target = args[0].(gen.PID)
	s.Send(s.PID(), "domonitor")
	return nil
}

func (s *t8demonitorSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "domonitor" {
			return s.MonitorPID(s.target)
		}
		if m == "demonitor" {
			return s.DemonitorPID(s.target)
		}
	case gen.MessageDownPID:
		return gen.TerminateReasonNormal
	}
	return nil
}
func (s *t8demonitorSub) Terminate(reason error) {
	s.Log().Warning("terminated: %v", reason)
}

// Test 1.3: Multiple links to same ProcessID
func TestT8MultipleLinksToSameProcessID(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true
	// opts1.Log.Level = gen.LogLevelTrace

	node1, err := ergo.StartNode("n1pid@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true
	// opts2.Log.Level = gen.LogLevelTrace

	node2, err := ergo.StartNode("n2pid@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn target with registered name
	targetPID, err := node2.SpawnRegister("namedTarget", factory_t8target, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	targetProcessID := gen.ProcessID{Node: node2.Name(), Name: "namedTarget"}

	// Spawn 3 subscribers linking to ProcessID
	sub1, err := node1.Spawn(factory_t8linkProcessIDSub, gen.ProcessOptions{}, targetProcessID)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8linkProcessIDSub, gen.ProcessOptions{}, targetProcessID)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8linkProcessIDSub, gen.ProcessOptions{}, targetProcessID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(600 * time.Millisecond)

	// Verify all linked
	info1, _ := node1.ProcessInfo(sub1)
	if len(info1.LinksProcessID) != 1 {
		t.Fatal("sub1 should be linked to ProcessID", info1)
	}

	info2, _ := node1.ProcessInfo(sub2)
	if len(info2.LinksProcessID) != 1 {
		t.Fatal("sub2 should be linked to ProcessID", info2)
	}

	info3, _ := node1.ProcessInfo(sub3)
	if len(info3.LinksProcessID) != 1 {
		t.Fatal("sub3 should be linked to ProcessID", info3)
	}

	// Terminate target
	node2.SendExit(targetPID, fmt.Errorf("test"))
	time.Sleep(300 * time.Millisecond)

	// All should terminate
	if _, err := node1.ProcessInfo(sub1); err != gen.ErrProcessUnknown {
		t.Error("sub1 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub2); err != gen.ErrProcessUnknown {
		t.Error("sub2 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub3); err != gen.ErrProcessUnknown {
		t.Error("sub3 should be terminated")
	}
}

func factory_t8linkProcessIDSub() gen.ProcessBehavior {
	return &t8linkProcessIDSub{}
}

type t8linkProcessIDSub struct {
	act.Actor
	target gen.ProcessID
}

func (s *t8linkProcessIDSub) Init(args ...any) error {
	s.target = args[0].(gen.ProcessID)
	s.SetTrapExit(true)
	s.Send(s.PID(), "dolink")
	return nil
}

func (s *t8linkProcessIDSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "dolink" {
			err := s.LinkProcessID(s.target)
			if err == nil {
				s.Log().Info("linked to ProcessID %v", s.target)
			} else {
				s.Log().Error("failed to link to ProcessID %v: %v", s.target, err)
			}
			return err
		}
	case gen.MessageExitProcessID:
		return gen.TerminateReasonNormal
	}
	return nil
}

// Test 1.4: Multiple links to same Alias
func TestT8MultipleLinksToSameAlias(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1a@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2a@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn target and create alias
	targetPID, err := node2.Spawn(factory_t8targetWithAlias, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Get alias from target (retry if not ready)
	var targetAlias gen.Alias
	for i := 0; i < 10; i++ {
		alias, err := node1.Call(targetPID, "getalias")
		if err != nil {
			t.Fatal(err)
		}
		targetAlias = alias.(gen.Alias)
		if targetAlias.Node != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if targetAlias.Node == "" {
		t.Fatal("alias not created")
	}

	// Spawn 3 subscribers linking to alias
	sub1, err := node1.Spawn(factory_t8linkAliasSub, gen.ProcessOptions{}, targetAlias)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8linkAliasSub, gen.ProcessOptions{}, targetAlias)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8linkAliasSub, gen.ProcessOptions{}, targetAlias)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify all linked
	info1, _ := node1.ProcessInfo(sub1)
	if len(info1.LinksAlias) != 1 {
		t.Fatal("sub1 should be linked to Alias")
	}

	// Terminate target
	node2.SendExit(targetPID, fmt.Errorf("test"))
	time.Sleep(300 * time.Millisecond)

	// All should terminate
	if _, err := node1.ProcessInfo(sub1); err != gen.ErrProcessUnknown {
		t.Error("sub1 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub2); err != gen.ErrProcessUnknown {
		t.Error("sub2 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub3); err != gen.ErrProcessUnknown {
		t.Error("sub3 should be terminated")
	}
}

func factory_t8targetWithAlias() gen.ProcessBehavior {
	return &t8targetWithAlias{}
}

type t8targetWithAlias struct {
	act.Actor
	alias gen.Alias
}

func (t *t8targetWithAlias) Init(args ...any) error {
	// Can't create alias in Init - send message to self
	t.Send(t.PID(), "createalias")
	return nil
}

func (t *t8targetWithAlias) HandleMessage(from gen.PID, message any) error {
	if message == "createalias" {
		alias, err := t.CreateAlias()
		if err != nil {
			return err
		}
		t.alias = alias
	}
	return nil
}

func (t *t8targetWithAlias) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == "getalias" {
		// Return alias even if empty - caller will check
		return t.alias, nil
	}
	return nil, nil
}

func factory_t8linkAliasSub() gen.ProcessBehavior {
	return &t8linkAliasSub{}
}

type t8linkAliasSub struct {
	act.Actor
	target gen.Alias
}

func (s *t8linkAliasSub) Init(args ...any) error {
	s.target = args[0].(gen.Alias)
	s.SetTrapExit(true)
	s.Send(s.PID(), "dolink")
	return nil
}

func (s *t8linkAliasSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "dolink" {
			return s.LinkAlias(s.target)
		}
	case gen.MessageExitAlias:
		return gen.TerminateReasonNormal
	}
	return nil
}

// Test 1.5: Multiple links to same Event
func TestT8MultipleLinksToSameEvent(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1e@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2e@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn event producer on node2
	producerPID, err := node2.Spawn(factory_t8eventProducer, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for event to be registered
	time.Sleep(300 * time.Millisecond)

	// Get event from producer (retry if not ready)
	var targetEvent gen.Event
	for i := 0; i < 10; i++ {
		event, err := node1.Call(producerPID, "getevent")
		if err != nil {
			t.Fatal(err)
		}
		targetEvent = event.(gen.Event)
		if targetEvent.Node != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if targetEvent.Node == "" {
		t.Fatal("event not created")
	}

	t.Logf("Got event: %v", targetEvent)

	// Spawn 3 subscribers linking to event
	sub1, err := node1.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify all linked to event
	info1, err := node1.ProcessInfo(sub1)
	if err != nil {
		t.Fatalf("failed to get info for sub1: %v", err)
	}
	if len(info1.LinksEvent) != 1 {
		t.Fatalf("sub1 should be linked to event, got: %d links", len(info1.LinksEvent))
	}

	info2, err := node1.ProcessInfo(sub2)
	if err != nil {
		t.Fatalf("failed to get info for sub2: %v (probably terminated due to LinkEvent error)", err)
	}
	if len(info2.LinksEvent) != 1 {
		t.Fatalf("sub2 should be linked to event, got: %d links", len(info2.LinksEvent))
	}

	info3, err := node1.ProcessInfo(sub3)
	if err != nil {
		t.Fatalf("failed to get info for sub3: %v", err)
	}
	if len(info3.LinksEvent) != 1 {
		t.Fatalf("sub3 should be linked to event, got: %d links", len(info3.LinksEvent))
	}

	// Terminate producer
	node2.SendExit(producerPID, fmt.Errorf("test"))
	time.Sleep(300 * time.Millisecond)

	// All should terminate
	if _, err := node1.ProcessInfo(sub1); err != gen.ErrProcessUnknown {
		t.Error("sub1 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub2); err != gen.ErrProcessUnknown {
		t.Error("sub2 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub3); err != gen.ErrProcessUnknown {
		t.Error("sub3 should be terminated")
	}
}

func factory_t8eventProducer() gen.ProcessBehavior {
	return &t8eventProducer{}
}

type t8eventProducer struct {
	act.Actor
	event gen.Event
	token gen.Ref
}

func (p *t8eventProducer) Init(args ...any) error {
	// Can't register event in Init
	p.Send(p.PID(), "registerevent")
	return nil
}

func (p *t8eventProducer) HandleMessage(from gen.PID, message any) error {
	if message == "registerevent" {
		token, err := p.RegisterEvent("testEvent", gen.EventOptions{Buffer: 2})
		if err != nil {
			return err
		}
		p.token = token
		p.event = gen.Event{Node: p.Node().Name(), Name: "testEvent"}

		// Send initial events to buffer
		p.SendEvent("testEvent", token, "event1")
		p.SendEvent("testEvent", token, "event2")
	}
	return nil
}

func (p *t8eventProducer) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == "getevent" {
		// Return event even if empty - caller will check
		return p.event, nil
	}
	return nil, nil
}

func factory_t8linkEventSub() gen.ProcessBehavior {
	return &t8linkEventSub{}
}

type t8linkEventSub struct {
	act.Actor
	target     gen.Event
	lastEvents []gen.MessageEvent
}

func (s *t8linkEventSub) Init(args ...any) error {
	s.target = args[0].(gen.Event)
	s.SetTrapExit(true)
	s.Send(s.PID(), "dolink")
	return nil
}

func (s *t8linkEventSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "dolink" {
			lastEvents, err := s.LinkEvent(s.target)
			if err != nil {
				s.Log().Error("LinkEvent failed: %v (target: %v)", err, s.target)
				return err
			}
			s.lastEvents = lastEvents
			s.Log().Info("Linked to event, got %d last events", len(lastEvents))
			return nil
		}
		if m == "unlink" {
			return s.UnlinkEvent(s.target)
		}
	case gen.MessageExitEvent:
		return gen.TerminateReasonNormal
	}
	return nil
}

// Test 3.1: Process with links terminates
// Cleanup removes local registration
// Remote unlink sent only if last subscriber
func TestT8ProcessTerminationWithLinks(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1pt@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2pt@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	targetPID, err := node2.Spawn(factory_t8target, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Spawn 2 subscribers
	sub1, err := node1.Spawn(factory_t8linkSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8linkSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Kill sub1 (not last - no remote unlink)
	node1.Kill(sub1)
	time.Sleep(100 * time.Millisecond)

	// sub2 still alive and linked
	info2, _ := node1.ProcessInfo(sub2)
	if len(info2.LinksPID) != 1 {
		t.Error("sub2 should still be linked")
	}

	// Target still alive (no remote unlink sent)
	if _, err := node2.ProcessInfo(targetPID); err != nil {
		t.Error("target should still be alive")
	}

	// Kill sub2 (last - remote unlink sent)
	node1.Kill(sub2)
	time.Sleep(100 * time.Millisecond)

	// Target should still be alive (unlink doesn't terminate target)
	if _, err := node2.ProcessInfo(targetPID); err != nil {
		t.Error("target should still be alive after unlink")
	}
}

// Test 3.2: Node down scenario
func TestT8NodeDown(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1nd@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2nd@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	targetPID, err := node2.Spawn(factory_t8target, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Spawn 3 subscribers on node1
	sub1, err := node1.Spawn(factory_t8linkSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8linkSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8linkSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify all linked
	info1, _ := node1.ProcessInfo(sub1)
	if len(info1.LinksPID) != 1 {
		t.Fatal("sub1 should be linked")
	}

	// Stop node2 (connection lost)
	node2.Stop()
	time.Sleep(300 * time.Millisecond)

	// All subs should terminate (got ErrNoConnection)
	if _, err := node1.ProcessInfo(sub1); err != gen.ErrProcessUnknown {
		t.Error("sub1 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub2); err != gen.ErrProcessUnknown {
		t.Error("sub2 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub3); err != gen.ErrProcessUnknown {
		t.Error("sub3 should be terminated")
	}
}

// Test 3.3: Duplicate operations
func TestT8DuplicateLink(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1dup@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2dup@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	targetPID, err := node2.Spawn(factory_t8target, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Spawn subscriber
	sub, err := node1.Spawn(factory_t8dupLinkSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Should be linked once
	info, _ := node1.ProcessInfo(sub)
	if len(info.LinksPID) != 1 {
		t.Fatal("should be linked once")
	}

	// Try to link again - should get error
	result := make(chan error, 1)
	node1.Send(sub, result)

	select {
	case err := <-result:
		if err != gen.ErrTargetExist {
			t.Errorf("expected ErrTargetExist, got: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for duplicate link result")
	}
}

func factory_t8dupLinkSub() gen.ProcessBehavior {
	return &t8dupLinkSub{}
}

type t8dupLinkSub struct {
	act.Actor
	target gen.PID
	linked bool
}

func (s *t8dupLinkSub) Init(args ...any) error {
	s.target = args[0].(gen.PID)
	s.SetTrapExit(true)
	s.Send(s.PID(), "dolink")
	return nil
}

func (s *t8dupLinkSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "dolink" {
			if err := s.LinkPID(s.target); err != nil {
				return err
			}
			s.linked = true
			return nil
		}
	case chan error:
		// Try to link again
		err := s.LinkPID(s.target)
		m <- err
		return nil
	case gen.MessageExitPID:
		return gen.TerminateReasonNormal
	}
	return nil
}

// Test 3.4: Mixed link and monitor
func TestT8MixedLinkMonitor(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1mix@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2mix@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	targetPID, err := node2.Spawn(factory_t8target, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Spawn process that both links AND monitors
	sub, err := node1.Spawn(factory_t8mixedSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify both link and monitor
	info, _ := node1.ProcessInfo(sub)
	if len(info.LinksPID) != 1 || len(info.MonitorsPID) != 1 {
		t.Fatal("should have both link and monitor")
	}

	// Terminate target
	node2.SendExit(targetPID, fmt.Errorf("test"))
	time.Sleep(300 * time.Millisecond)

	// Should terminate (exit signal from link)
	if _, err := node1.ProcessInfo(sub); err != gen.ErrProcessUnknown {
		t.Error("sub should be terminated")
	}
}

func factory_t8mixedSub() gen.ProcessBehavior {
	return &t8mixedSub{}
}

type t8mixedSub struct {
	act.Actor
	target gen.PID
}

func (s *t8mixedSub) Init(args ...any) error {
	s.target = args[0].(gen.PID)
	s.SetTrapExit(true)
	s.Send(s.PID(), "doboth")
	return nil
}

func (s *t8mixedSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "doboth" {
			if err := s.LinkPID(s.target); err != nil {
				return err
			}
			return s.MonitorPID(s.target)
		}
	case gen.MessageExitPID:
		return gen.TerminateReasonNormal
	case gen.MessageDownPID:
		return gen.TerminateReasonNormal
	}
	return nil
}

// Test 4: Event last messages
func TestT8EventLastMessages(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1evl@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2evl@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn producer with buffer
	producerPID, err := node2.Spawn(factory_t8eventProducer, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	// Get event (retry if not ready)
	var targetEvent gen.Event
	for i := 0; i < 10; i++ {
		event, err := node1.Call(producerPID, "getevent")
		if err != nil {
			t.Fatal(err)
		}
		targetEvent = event.(gen.Event)
		if targetEvent.Node != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if targetEvent.Node == "" {
		t.Fatal("event not created")
	}

	// Spawn 3 subscribers
	resultChan := make(chan int, 3)

	sub1, err := node1.Spawn(factory_t8eventLastSub, gen.ProcessOptions{}, targetEvent, resultChan)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	sub2, err := node1.Spawn(factory_t8eventLastSub, gen.ProcessOptions{}, targetEvent, resultChan)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	sub3, err := node1.Spawn(factory_t8eventLastSub, gen.ProcessOptions{}, targetEvent, resultChan)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Collect results
	results := make([]int, 0, 3)
	for i := 0; i < 3; i++ {
		select {
		case count := <-resultChan:
			results = append(results, count)
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for results")
		}
	}

	// First subscriber gets 2 last events
	if results[0] != 2 {
		t.Errorf("first subscriber should get 2 last events, got: %d", results[0])
	}

	// Second and third might get 0 or 2 depending on optimization
	// For now just check they got a result

	// Cleanup
	node1.Kill(sub1)
	node1.Kill(sub2)
	node1.Kill(sub3)
}

func factory_t8eventLastSub() gen.ProcessBehavior {
	return &t8eventLastSub{}
}

type t8eventLastSub struct {
	act.Actor
	target     gen.Event
	resultChan chan int
}

func (s *t8eventLastSub) Init(args ...any) error {
	s.target = args[0].(gen.Event)
	s.resultChan = args[1].(chan int)
	s.Send(s.PID(), "dolink")
	return nil
}

func (s *t8eventLastSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "dolink" {
			lastEvents, err := s.LinkEvent(s.target)
			if err != nil {
				return err
			}
			// Report count of last events
			s.resultChan <- len(lastEvents)
			return nil
		}
	}
	return nil
}

// Test 5: Cross-node subscriptions
func TestT8CrossNodeSubscriptions(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1x@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2x@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	opts3 := gen.NodeOptions{}
	opts3.Network.Cookie = "test123"
	opts3.Log.DefaultLogger.Disable = true

	node3, err := ergo.StartNode("n3x@localhost", opts3)
	if err != nil {
		t.Fatal(err)
	}
	defer node3.Stop()

	// Connect all nodes
	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}
	if _, err := node3.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn target on node2
	targetPID, err := node2.Spawn(factory_t8target, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Spawn subscribers on node1
	sub1, err := node1.Spawn(factory_t8linkSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8linkSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	// Spawn subscriber on node3
	sub3, err := node3.Spawn(factory_t8linkSub, gen.ProcessOptions{}, targetPID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify all linked
	info1, _ := node1.ProcessInfo(sub1)
	if len(info1.LinksPID) != 1 {
		t.Fatal("sub1 should be linked")
	}

	info3, _ := node3.ProcessInfo(sub3)
	if len(info3.LinksPID) != 1 {
		t.Fatal("sub3 should be linked")
	}

	// Terminate target
	node2.SendExit(targetPID, fmt.Errorf("test"))
	time.Sleep(300 * time.Millisecond)

	// All should terminate
	if _, err := node1.ProcessInfo(sub1); err != gen.ErrProcessUnknown {
		t.Error("sub1 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub2); err != gen.ErrProcessUnknown {
		t.Error("sub2 should be terminated")
	}
	if _, err := node3.ProcessInfo(sub3); err != gen.ErrProcessUnknown {
		t.Error("sub3 should be terminated")
	}
}

// Test 6: Event delivery to all subscribers
func TestT8EventDelivery(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1ev@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2ev@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn producer
	producerPID, err := node2.Spawn(factory_t8eventPublisher, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	// Get event (retry if not ready)
	var targetEvent gen.Event
	for i := 0; i < 10; i++ {
		event, err := node1.Call(producerPID, "getevent")
		if err != nil {
			t.Fatal(err)
		}
		targetEvent = event.(gen.Event)
		if targetEvent.Node != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if targetEvent.Node == "" {
		t.Fatal("event not created")
	}

	// Spawn 3 subscribers that will receive events
	eventChan := make(chan string, 10)

	sub1, err := node1.Spawn(factory_t8eventReceiver, gen.ProcessOptions{}, targetEvent, eventChan)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8eventReceiver, gen.ProcessOptions{}, targetEvent, eventChan)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8eventReceiver, gen.ProcessOptions{}, targetEvent, eventChan)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Tell producer to publish event
	node2.Send(producerPID, "publish")

	// Wait for events
	time.Sleep(300 * time.Millisecond)

	// Should have received 3 events (one per subscriber)
	received := 0
	for {
		select {
		case <-eventChan:
			received++
		case <-time.After(100 * time.Millisecond):
			goto done
		}
	}
done:
	if received != 3 {
		t.Errorf("expected 3 events, got: %d", received)
	}

	// Cleanup
	node1.Kill(sub1)
	node1.Kill(sub2)
	node1.Kill(sub3)
}

func factory_t8eventPublisher() gen.ProcessBehavior {
	return &t8eventPublisher{}
}

type t8eventPublisher struct {
	act.Actor
	event gen.Event
	token gen.Ref
}

func (p *t8eventPublisher) Init(args ...any) error {
	p.Send(p.PID(), "registerevent")
	return nil
}

func (p *t8eventPublisher) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == "getevent" {
		// Return event even if empty - caller will check
		return p.event, nil
	}
	return nil, nil
}

func (p *t8eventPublisher) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case string:
		if message == "registerevent" {
			token, err := p.RegisterEvent("pubEvent", gen.EventOptions{Buffer: 0})
			if err != nil {
				return err
			}
			p.token = token
			p.event = gen.Event{Node: p.Node().Name(), Name: "pubEvent"}
		} else if message == "publish" {
			p.SendEvent("pubEvent", p.token, "test-event-data")
		}
	}
	return nil
}

func factory_t8eventReceiver() gen.ProcessBehavior {
	return &t8eventReceiver{}
}

type t8eventReceiver struct {
	act.Actor
	target    gen.Event
	eventChan chan string
}

func (s *t8eventReceiver) Init(args ...any) error {
	s.target = args[0].(gen.Event)
	s.eventChan = args[1].(chan string)
	s.Send(s.PID(), "dolink")
	return nil
}

func (s *t8eventReceiver) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "dolink" {
			_, err := s.LinkEvent(s.target)
			return err
		}
	}
	return nil
}

func (s *t8eventReceiver) HandleEvent(message gen.MessageEvent) error {
	// Received event - report it
	s.eventChan <- message.Message.(string)
	return nil
}

// Test 7.1: Producer notification - local subscribers only
func TestT8ProducerNotificationLocal(t *testing.T) {
	opts := gen.NodeOptions{}
	opts.Network.Cookie = "test123"
	opts.Log.DefaultLogger.Disable = true

	node, err := ergo.StartNode("nlocal@localhost", opts)
	if err != nil {
		t.Fatal(err)
	}
	defer node.Stop()

	// Spawn producer with notify enabled
	notifyChan := make(chan string, 10)
	producerPID, err := node.Spawn(factory_t8notifyProducer, gen.ProcessOptions{}, notifyChan)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Get event
	event, err := node.Call(producerPID, "getevent")
	if err != nil {
		t.Fatal(err)
	}
	targetEvent := event.(gen.Event)

	// Spawn first subscriber - should trigger EventStart
	sub1, err := node.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Check notification
	select {
	case msg := <-notifyChan:
		if msg != "start" {
			t.Errorf("expected 'start', got: %s", msg)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for EventStart notification")
	}

	// Spawn second subscriber - should NOT trigger EventStart
	sub2, err := node.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Should be no notification
	select {
	case msg := <-notifyChan:
		t.Errorf("unexpected notification: %s", msg)
	case <-time.After(300 * time.Millisecond):
		// Good - no notification
	}

	// Unlink first subscriber - should NOT trigger EventStop
	node.Send(sub1, "unlink")
	time.Sleep(200 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		t.Errorf("unexpected notification after first unlink: %s", msg)
	case <-time.After(300 * time.Millisecond):
		// Good
	}

	// Unlink second subscriber (last) - should trigger EventStop
	node.Send(sub2, "unlink")
	time.Sleep(200 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		if msg != "stop" {
			t.Errorf("expected 'stop', got: %s", msg)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for EventStop notification")
	}
}

func factory_t8notifyProducer() gen.ProcessBehavior {
	return &t8notifyProducer{}
}

type t8notifyProducer struct {
	act.Actor
	event      gen.Event
	token      gen.Ref
	notifyChan chan string
}

func (p *t8notifyProducer) Init(args ...any) error {
	p.notifyChan = args[0].(chan string)
	p.Send(p.PID(), "registerevent")
	return nil
}

func (p *t8notifyProducer) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "registerevent" {
			token, err := p.RegisterEvent("notifyEvent", gen.EventOptions{Buffer: 0, Notify: true})
			if err != nil {
				return err
			}
			p.token = token
			p.event = gen.Event{Node: p.Node().Name(), Name: "notifyEvent"}
		}
	case gen.MessageEventStart:
		p.notifyChan <- "start"
	case gen.MessageEventStop:
		p.notifyChan <- "stop"
	}
	return nil
}

func (p *t8notifyProducer) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == "getevent" {
		return p.event, nil
	}
	return nil, nil
}

// Test 7.2: Producer notification - remote subscribers only
func TestT8ProducerNotificationRemote(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true
	opts1.Log.Level = gen.LogLevelTrace

	node1, err := ergo.StartNode("n1pn@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true
	opts2.Log.Level = gen.LogLevelTrace

	node2, err := ergo.StartNode("n2pn@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn producer on node2 with notify
	notifyChan := make(chan string, 10)
	producerPID, err := node2.Spawn(factory_t8notifyProducer, gen.ProcessOptions{}, notifyChan)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	event, err := node2.Call(producerPID, "getevent")
	if err != nil {
		t.Fatal(err)
	}
	targetEvent := event.(gen.Event)

	// Spawn first subscriber on node1 - should trigger EventStart
	sub1, err := node1.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		if msg != "start" {
			t.Errorf("expected 'start', got: %s", msg)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for EventStart")
	}

	// Spawn second subscriber - no notification
	sub2, err := node1.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		t.Errorf("unexpected notification: %s", msg)
	case <-time.After(300 * time.Millisecond):
		// Good
	}

	// Kill first subscriber - no EventStop
	node1.Kill(sub1)
	time.Sleep(200 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		t.Errorf("unexpected notification: %s", msg)
	case <-time.After(300 * time.Millisecond):
		// Good
	}

	// Kill second subscriber (last) - should trigger EventStop
	node1.Kill(sub2)
	time.Sleep(500 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		if msg != "stop" {
			t.Errorf("expected 'stop', got: %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for EventStop")
	}
}

// Test 7.3: Producer notification - mixed local and remote subscribers
func TestT8ProducerNotificationMixed(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1mix@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2mix@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn producer on node1 with notify
	notifyChan := make(chan string, 10)
	producerPID, err := node1.Spawn(factory_t8notifyProducer, gen.ProcessOptions{}, notifyChan)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	event, err := node1.Call(producerPID, "getevent")
	if err != nil {
		t.Fatal(err)
	}
	targetEvent := event.(gen.Event)

	// Spawn local subscriber first - should trigger EventStart
	localSub, err := node1.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		if msg != "start" {
			t.Errorf("expected 'start', got: %s", msg)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for EventStart")
	}

	// Spawn remote subscriber - no notification
	remoteSub, err := node2.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify both are linked
	infoLocal, _ := node1.ProcessInfo(localSub)
	if len(infoLocal.LinksEvent) != 1 {
		t.Fatal("localSub should be linked")
	}

	infoRemote, _ := node2.ProcessInfo(remoteSub)
	if len(infoRemote.LinksEvent) != 1 {
		t.Fatal("remoteSub should be linked")
	}

	select {
	case msg := <-notifyChan:
		t.Errorf("unexpected notification: %s", msg)
	case <-time.After(300 * time.Millisecond):
		// Good
	}

	// Kill local subscriber - no EventStop (remote still subscribed)
	node1.Kill(localSub)
	time.Sleep(200 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		t.Errorf("unexpected notification: %s", msg)
	case <-time.After(300 * time.Millisecond):
		// Good
	}

	// Kill remote subscriber (last) - should trigger EventStop
	node2.Kill(remoteSub)
	time.Sleep(1 * time.Second)

	select {
	case msg := <-notifyChan:
		if msg != "stop" {
			t.Errorf("expected 'stop', got: %s", msg)
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for EventStop")
	}
}

// Test 7.4: Producer notification - multiple remote nodes
func TestT8ProducerNotificationMultiNode(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1mn@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2mn@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	opts3 := gen.NodeOptions{}
	opts3.Network.Cookie = "test123"
	opts3.Log.DefaultLogger.Disable = true

	node3, err := ergo.StartNode("n3mn@localhost", opts3)
	if err != nil {
		t.Fatal(err)
	}
	defer node3.Stop()

	// Connect all
	if _, err := node2.Network().GetNode(node1.Name()); err != nil {
		t.Fatal(err)
	}
	if _, err := node3.Network().GetNode(node1.Name()); err != nil {
		t.Fatal(err)
	}

	// Producer on node1
	notifyChan := make(chan string, 10)
	producerPID, err := node1.Spawn(factory_t8notifyProducer, gen.ProcessOptions{}, notifyChan)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	event, err := node1.Call(producerPID, "getevent")
	if err != nil {
		t.Fatal(err)
	}
	targetEvent := event.(gen.Event)

	// Spawn subscriber on node2 (first) - triggers EventStart
	sub2, err := node2.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		if msg != "start" {
			t.Errorf("expected 'start', got: %s", msg)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for EventStart")
	}

	// Spawn subscriber on node3 - no notification
	sub3, err := node3.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		t.Errorf("unexpected notification: %s", msg)
	case <-time.After(300 * time.Millisecond):
		// Good
	}

	// Kill sub on node2 - no EventStop (node3 still has subscriber)
	node2.Kill(sub2)
	time.Sleep(200 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		t.Errorf("unexpected notification: %s", msg)
	case <-time.After(300 * time.Millisecond):
		// Good
	}

	// Kill sub on node3 (last) - triggers EventStop
	node3.Kill(sub3)
	time.Sleep(500 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		if msg != "stop" {
			t.Errorf("expected 'stop', got: %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for EventStop")
	}
}

// Test 7.5: Producer notification - link and monitor mixed
func TestT8ProducerNotificationLinkMonitorMix(t *testing.T) {
	opts := gen.NodeOptions{}
	opts.Network.Cookie = "test123"
	opts.Log.DefaultLogger.Disable = true

	node, err := ergo.StartNode("nmix@localhost", opts)
	if err != nil {
		t.Fatal(err)
	}
	defer node.Stop()

	notifyChan := make(chan string, 10)
	producerPID, err := node.Spawn(factory_t8notifyProducer, gen.ProcessOptions{}, notifyChan)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	event, err := node.Call(producerPID, "getevent")
	if err != nil {
		t.Fatal(err)
	}
	targetEvent := event.(gen.Event)

	// Link first - triggers EventStart
	linkSub, err := node.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		if msg != "start" {
			t.Errorf("expected 'start', got: %s", msg)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for EventStart")
	}

	// Monitor second - no notification (already has link subscriber)
	monitorSub, err := node.Spawn(factory_t8monitorEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		t.Errorf("unexpected notification: %s", msg)
	case <-time.After(300 * time.Millisecond):
		// Good
	}

	// Kill link subscriber - no EventStop (monitor still active)
	node.Kill(linkSub)
	time.Sleep(200 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		t.Errorf("unexpected notification: %s", msg)
	case <-time.After(300 * time.Millisecond):
		// Good
	}

	// Kill monitor subscriber (last) - triggers EventStop
	node.Kill(monitorSub)
	time.Sleep(500 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		if msg != "stop" {
			t.Errorf("expected 'stop', got: %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for EventStop")
	}
}

func factory_t8monitorEventSub() gen.ProcessBehavior {
	return &t8monitorEventSub{}
}

type t8monitorEventSub struct {
	act.Actor
	target gen.Event
}

func (s *t8monitorEventSub) Init(args ...any) error {
	s.target = args[0].(gen.Event)
	s.Send(s.PID(), "domonitor")
	return nil
}

func (s *t8monitorEventSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "domonitor" {
			_, err := s.MonitorEvent(s.target)
			return err
		}
		if m == "demonitor" {
			return s.DemonitorEvent(s.target)
		}
	case gen.MessageDownEvent:
		return gen.TerminateReasonNormal
	}
	return nil
}

// Test 7.6: No notification when notify disabled
func TestT8ProducerNoNotification(t *testing.T) {
	opts := gen.NodeOptions{}
	opts.Network.Cookie = "test123"
	opts.Log.DefaultLogger.Disable = true

	node, err := ergo.StartNode("nnonot@localhost", opts)
	if err != nil {
		t.Fatal(err)
	}
	defer node.Stop()

	// Producer with notify=false
	notifyChan := make(chan string, 10)
	producerPID, err := node.Spawn(factory_t8noNotifyProducer, gen.ProcessOptions{}, notifyChan)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	event, err := node.Call(producerPID, "getevent")
	if err != nil {
		t.Fatal(err)
	}
	targetEvent := event.(gen.Event)

	// Spawn subscriber - should NOT trigger notification
	sub, err := node.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		t.Errorf("unexpected notification (notify disabled): %s", msg)
	case <-time.After(300 * time.Millisecond):
		// Good
	}

	// Kill subscriber - no notification
	node.Kill(sub)
	time.Sleep(200 * time.Millisecond)

	select {
	case msg := <-notifyChan:
		t.Errorf("unexpected notification: %s", msg)
	case <-time.After(300 * time.Millisecond):
		// Good
	}
}

func factory_t8noNotifyProducer() gen.ProcessBehavior {
	return &t8noNotifyProducer{}
}

type t8noNotifyProducer struct {
	act.Actor
	event      gen.Event
	token      gen.Ref
	notifyChan chan string
}

func (p *t8noNotifyProducer) Init(args ...any) error {
	p.notifyChan = args[0].(chan string)
	p.Send(p.PID(), "registerevent")
	return nil
}

func (p *t8noNotifyProducer) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "registerevent" {
			token, err := p.RegisterEvent("noNotifyEvent", gen.EventOptions{Buffer: 0, Notify: false})
			if err != nil {
				return err
			}
			p.token = token
			p.event = gen.Event{Node: p.Node().Name(), Name: "noNotifyEvent"}
		}
	case gen.MessageEventStart:
		p.notifyChan <- "start"
	case gen.MessageEventStop:
		p.notifyChan <- "stop"
	}
	return nil
}

func (p *t8noNotifyProducer) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == "getevent" {
		return p.event, nil
	}
	return nil, nil
}

// =============================================================================
// Test 1.2b: Multiple monitors to same ProcessID
// =============================================================================

func TestT8MultipleMonitorsToSameProcessID(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1mpid@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2mpid@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn target with registered name
	targetPID, err := node2.SpawnRegister("monitorTarget", factory_t8target, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	targetProcessID := gen.ProcessID{Node: node2.Name(), Name: "monitorTarget"}

	// Spawn 3 subscribers monitoring ProcessID
	sub1, err := node1.Spawn(factory_t8monitorProcessIDSub, gen.ProcessOptions{}, targetProcessID)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8monitorProcessIDSub, gen.ProcessOptions{}, targetProcessID)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8monitorProcessIDSub, gen.ProcessOptions{}, targetProcessID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(400 * time.Millisecond)

	// Verify all monitoring
	info1, _ := node1.ProcessInfo(sub1)
	if len(info1.MonitorsProcessID) != 1 {
		t.Fatal("sub1 should be monitoring ProcessID")
	}

	info2, _ := node1.ProcessInfo(sub2)
	if len(info2.MonitorsProcessID) != 1 {
		t.Fatal("sub2 should be monitoring ProcessID")
	}

	info3, _ := node1.ProcessInfo(sub3)
	if len(info3.MonitorsProcessID) != 1 {
		t.Fatal("sub3 should be monitoring ProcessID")
	}

	// Terminate target
	node2.SendExit(targetPID, fmt.Errorf("test"))
	time.Sleep(300 * time.Millisecond)

	// All should terminate
	if _, err := node1.ProcessInfo(sub1); err != gen.ErrProcessUnknown {
		t.Error("sub1 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub2); err != gen.ErrProcessUnknown {
		t.Error("sub2 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub3); err != gen.ErrProcessUnknown {
		t.Error("sub3 should be terminated")
	}
}

func factory_t8monitorProcessIDSub() gen.ProcessBehavior {
	return &t8monitorProcessIDSub{}
}

type t8monitorProcessIDSub struct {
	act.Actor
	target gen.ProcessID
}

func (s *t8monitorProcessIDSub) Init(args ...any) error {
	s.target = args[0].(gen.ProcessID)
	s.Send(s.PID(), "domonitor")
	return nil
}

func (s *t8monitorProcessIDSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "domonitor" {
			return s.MonitorProcessID(s.target)
		}
		if m == "demonitor" {
			return s.DemonitorProcessID(s.target)
		}
	case gen.MessageDownProcessID:
		return gen.TerminateReasonNormal
	}
	return nil
}

// =============================================================================
// Test 1.2c: Multiple monitors to same Alias
// =============================================================================

func TestT8MultipleMonitorsToSameAlias(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1ma@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2ma@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn target and create alias
	targetPID, err := node2.Spawn(factory_t8targetWithAlias, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Get alias from target
	var targetAlias gen.Alias
	for i := 0; i < 10; i++ {
		alias, err := node1.Call(targetPID, "getalias")
		if err != nil {
			t.Fatal(err)
		}
		targetAlias = alias.(gen.Alias)
		if targetAlias.Node != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if targetAlias.Node == "" {
		t.Fatal("alias not created")
	}

	// Spawn 3 subscribers monitoring alias
	sub1, err := node1.Spawn(factory_t8monitorAliasSub, gen.ProcessOptions{}, targetAlias)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8monitorAliasSub, gen.ProcessOptions{}, targetAlias)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8monitorAliasSub, gen.ProcessOptions{}, targetAlias)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify all monitoring
	info1, _ := node1.ProcessInfo(sub1)
	if len(info1.MonitorsAlias) != 1 {
		t.Fatal("sub1 should be monitoring Alias")
	}

	// Terminate target
	node2.SendExit(targetPID, fmt.Errorf("test"))
	time.Sleep(300 * time.Millisecond)

	// All should terminate
	if _, err := node1.ProcessInfo(sub1); err != gen.ErrProcessUnknown {
		t.Error("sub1 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub2); err != gen.ErrProcessUnknown {
		t.Error("sub2 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub3); err != gen.ErrProcessUnknown {
		t.Error("sub3 should be terminated")
	}
}

func factory_t8monitorAliasSub() gen.ProcessBehavior {
	return &t8monitorAliasSub{}
}

type t8monitorAliasSub struct {
	act.Actor
	target gen.Alias
}

func (s *t8monitorAliasSub) Init(args ...any) error {
	s.target = args[0].(gen.Alias)
	s.Send(s.PID(), "domonitor")
	return nil
}

func (s *t8monitorAliasSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "domonitor" {
			return s.MonitorAlias(s.target)
		}
		if m == "demonitor" {
			return s.DemonitorAlias(s.target)
		}
	case gen.MessageDownAlias:
		return gen.TerminateReasonNormal
	}
	return nil
}

// =============================================================================
// Test 1.2d: Multiple monitors to same Event
// =============================================================================

func TestT8MultipleMonitorsToSameEvent(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1me@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2me@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn event producer
	producerPID, err := node2.Spawn(factory_t8eventProducer, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	// Get event
	var targetEvent gen.Event
	for i := 0; i < 10; i++ {
		event, err := node1.Call(producerPID, "getevent")
		if err != nil {
			t.Fatal(err)
		}
		targetEvent = event.(gen.Event)
		if targetEvent.Node != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if targetEvent.Node == "" {
		t.Fatal("event not created")
	}

	// Spawn 3 subscribers monitoring event
	sub1, err := node1.Spawn(factory_t8monitorEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8monitorEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8monitorEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify all monitoring
	info1, err := node1.ProcessInfo(sub1)
	if err != nil {
		t.Fatalf("failed to get info for sub1: %v", err)
	}
	if len(info1.MonitorsEvent) != 1 {
		t.Fatal("sub1 should be monitoring Event")
	}

	// Terminate producer
	node2.SendExit(producerPID, fmt.Errorf("test"))
	time.Sleep(300 * time.Millisecond)

	// All should terminate
	if _, err := node1.ProcessInfo(sub1); err != gen.ErrProcessUnknown {
		t.Error("sub1 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub2); err != gen.ErrProcessUnknown {
		t.Error("sub2 should be terminated")
	}
	if _, err := node1.ProcessInfo(sub3); err != gen.ErrProcessUnknown {
		t.Error("sub3 should be terminated")
	}
}

// =============================================================================
// Test 2.1b: Partial unlink for ProcessID
// =============================================================================

func TestT8PartialUnlinkProcessID(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1upid@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2upid@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn target with registered name
	targetPID, err := node2.SpawnRegister("unlinkTarget", factory_t8target, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	targetProcessID := gen.ProcessID{Node: node2.Name(), Name: "unlinkTarget"}

	// Spawn 3 subscribers linking to ProcessID
	sub1, err := node1.Spawn(factory_t8unlinkProcessIDSub, gen.ProcessOptions{}, targetProcessID)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8unlinkProcessIDSub, gen.ProcessOptions{}, targetProcessID)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8unlinkProcessIDSub, gen.ProcessOptions{}, targetProcessID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(400 * time.Millisecond)

	// Verify all linked
	info1, _ := node1.ProcessInfo(sub1)
	if len(info1.LinksProcessID) != 1 {
		t.Fatal("sub1 should be linked")
	}

	// P1 unlinks
	node1.Send(sub1, "unlink")
	time.Sleep(100 * time.Millisecond)

	info1, _ = node1.ProcessInfo(sub1)
	if len(info1.LinksProcessID) != 0 {
		t.Error("sub1 should not be linked after unlink")
	}

	// P2, P3 still alive
	if _, err := node1.ProcessInfo(sub2); err != nil {
		t.Error("sub2 should still be alive")
	}
	if _, err := node1.ProcessInfo(sub3); err != nil {
		t.Error("sub3 should still be alive")
	}

	// P2 unlinks
	node1.Send(sub2, "unlink")
	time.Sleep(100 * time.Millisecond)

	// P3 still alive
	if _, err := node1.ProcessInfo(sub3); err != nil {
		t.Error("sub3 should still be alive")
	}

	// P3 unlinks (last)
	node1.Send(sub3, "unlink")
	time.Sleep(100 * time.Millisecond)

	// Terminate target - nobody should get exit
	node2.SendExit(targetPID, fmt.Errorf("test"))
	time.Sleep(200 * time.Millisecond)

	// All subs still alive
	if _, err := node1.ProcessInfo(sub1); err != nil {
		t.Error("sub1 should still be alive")
	}
	if _, err := node1.ProcessInfo(sub2); err != nil {
		t.Error("sub2 should still be alive")
	}
	if _, err := node1.ProcessInfo(sub3); err != nil {
		t.Error("sub3 should still be alive")
	}
}

func factory_t8unlinkProcessIDSub() gen.ProcessBehavior {
	return &t8unlinkProcessIDSub{}
}

type t8unlinkProcessIDSub struct {
	act.Actor
	target gen.ProcessID
}

func (s *t8unlinkProcessIDSub) Init(args ...any) error {
	s.target = args[0].(gen.ProcessID)
	s.SetTrapExit(true)
	s.Send(s.PID(), "dolink")
	return nil
}

func (s *t8unlinkProcessIDSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "dolink" {
			return s.LinkProcessID(s.target)
		}
		if m == "unlink" {
			return s.UnlinkProcessID(s.target)
		}
	case gen.MessageExitProcessID:
		return gen.TerminateReasonNormal
	}
	return nil
}

// =============================================================================
// Test 2.1c: Partial unlink for Alias
// =============================================================================

func TestT8PartialUnlinkAlias(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1ua@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2ua@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn target with alias
	targetPID, err := node2.Spawn(factory_t8targetWithAlias, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Get alias
	var targetAlias gen.Alias
	for i := 0; i < 10; i++ {
		alias, err := node1.Call(targetPID, "getalias")
		if err != nil {
			t.Fatal(err)
		}
		targetAlias = alias.(gen.Alias)
		if targetAlias.Node != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if targetAlias.Node == "" {
		t.Fatal("alias not created")
	}

	// Spawn 3 subscribers
	sub1, err := node1.Spawn(factory_t8unlinkAliasSub, gen.ProcessOptions{}, targetAlias)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8unlinkAliasSub, gen.ProcessOptions{}, targetAlias)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8unlinkAliasSub, gen.ProcessOptions{}, targetAlias)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify all linked
	info1, _ := node1.ProcessInfo(sub1)
	if len(info1.LinksAlias) != 1 {
		t.Fatal("sub1 should be linked")
	}

	// P1 unlinks
	node1.Send(sub1, "unlink")
	time.Sleep(100 * time.Millisecond)

	info1, _ = node1.ProcessInfo(sub1)
	if len(info1.LinksAlias) != 0 {
		t.Error("sub1 should not be linked after unlink")
	}

	// P2 unlinks
	node1.Send(sub2, "unlink")
	time.Sleep(100 * time.Millisecond)

	// P3 unlinks (last)
	node1.Send(sub3, "unlink")
	time.Sleep(100 * time.Millisecond)

	// Terminate target - nobody should get exit
	node2.SendExit(targetPID, fmt.Errorf("test"))
	time.Sleep(200 * time.Millisecond)

	// All subs still alive
	if _, err := node1.ProcessInfo(sub1); err != nil {
		t.Error("sub1 should still be alive")
	}
	if _, err := node1.ProcessInfo(sub2); err != nil {
		t.Error("sub2 should still be alive")
	}
	if _, err := node1.ProcessInfo(sub3); err != nil {
		t.Error("sub3 should still be alive")
	}
}

func factory_t8unlinkAliasSub() gen.ProcessBehavior {
	return &t8unlinkAliasSub{}
}

type t8unlinkAliasSub struct {
	act.Actor
	target gen.Alias
}

func (s *t8unlinkAliasSub) Init(args ...any) error {
	s.target = args[0].(gen.Alias)
	s.SetTrapExit(true)
	s.Send(s.PID(), "dolink")
	return nil
}

func (s *t8unlinkAliasSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "dolink" {
			return s.LinkAlias(s.target)
		}
		if m == "unlink" {
			return s.UnlinkAlias(s.target)
		}
	case gen.MessageExitAlias:
		return gen.TerminateReasonNormal
	}
	return nil
}

// =============================================================================
// Test 2.1d: Partial unlink for Event
// =============================================================================

func TestT8PartialUnlinkEvent(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1ue@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2ue@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn event producer
	producerPID, err := node2.Spawn(factory_t8eventProducer, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	// Get event
	var targetEvent gen.Event
	for i := 0; i < 10; i++ {
		event, err := node1.Call(producerPID, "getevent")
		if err != nil {
			t.Fatal(err)
		}
		targetEvent = event.(gen.Event)
		if targetEvent.Node != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if targetEvent.Node == "" {
		t.Fatal("event not created")
	}

	// Spawn 3 subscribers
	sub1, err := node1.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8linkEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify all linked
	info1, err := node1.ProcessInfo(sub1)
	if err != nil {
		t.Fatalf("failed to get info for sub1: %v", err)
	}
	if len(info1.LinksEvent) != 1 {
		t.Fatal("sub1 should be linked to event")
	}

	// P1 unlinks
	node1.Send(sub1, "unlink")
	time.Sleep(100 * time.Millisecond)

	info1, _ = node1.ProcessInfo(sub1)
	if len(info1.LinksEvent) != 0 {
		t.Error("sub1 should not be linked after unlink")
	}

	// P2 unlinks
	node1.Send(sub2, "unlink")
	time.Sleep(100 * time.Millisecond)

	// P3 unlinks (last)
	node1.Send(sub3, "unlink")
	time.Sleep(100 * time.Millisecond)

	// Terminate producer - nobody should get exit
	node2.SendExit(producerPID, fmt.Errorf("test"))
	time.Sleep(200 * time.Millisecond)

	// All subs still alive
	if _, err := node1.ProcessInfo(sub1); err != nil {
		t.Error("sub1 should still be alive")
	}
	if _, err := node1.ProcessInfo(sub2); err != nil {
		t.Error("sub2 should still be alive")
	}
	if _, err := node1.ProcessInfo(sub3); err != nil {
		t.Error("sub3 should still be alive")
	}
}

// =============================================================================
// Test 2.2b: Partial demonitor for ProcessID
// =============================================================================

func TestT8PartialDemonitorProcessID(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1dpid@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2dpid@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn target with registered name
	targetPID, err := node2.SpawnRegister("demonitorTarget", factory_t8target, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	targetProcessID := gen.ProcessID{Node: node2.Name(), Name: "demonitorTarget"}

	// Spawn 3 subscribers
	sub1, err := node1.Spawn(factory_t8monitorProcessIDSub, gen.ProcessOptions{}, targetProcessID)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8monitorProcessIDSub, gen.ProcessOptions{}, targetProcessID)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8monitorProcessIDSub, gen.ProcessOptions{}, targetProcessID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(400 * time.Millisecond)

	// Verify all monitoring
	info1, _ := node1.ProcessInfo(sub1)
	if len(info1.MonitorsProcessID) != 1 {
		t.Fatal("sub1 should be monitoring")
	}

	// P1 demonitors
	node1.Send(sub1, "demonitor")
	time.Sleep(100 * time.Millisecond)

	info1, _ = node1.ProcessInfo(sub1)
	if len(info1.MonitorsProcessID) != 0 {
		t.Error("sub1 should not be monitoring after demonitor")
	}

	// P2 demonitors
	node1.Send(sub2, "demonitor")
	time.Sleep(100 * time.Millisecond)

	// P3 demonitors (last)
	node1.Send(sub3, "demonitor")
	time.Sleep(100 * time.Millisecond)

	// Terminate target - nobody should get down
	node2.SendExit(targetPID, fmt.Errorf("test"))
	time.Sleep(200 * time.Millisecond)

	// All subs still alive
	if _, err := node1.ProcessInfo(sub1); err != nil {
		t.Error("sub1 should still be alive")
	}
	if _, err := node1.ProcessInfo(sub2); err != nil {
		t.Error("sub2 should still be alive")
	}
	if _, err := node1.ProcessInfo(sub3); err != nil {
		t.Error("sub3 should still be alive")
	}
}

// =============================================================================
// Test 2.2c: Partial demonitor for Alias
// =============================================================================

func TestT8PartialDemonitorAlias(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1da@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2da@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn target with alias
	targetPID, err := node2.Spawn(factory_t8targetWithAlias, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Get alias
	var targetAlias gen.Alias
	for i := 0; i < 10; i++ {
		alias, err := node1.Call(targetPID, "getalias")
		if err != nil {
			t.Fatal(err)
		}
		targetAlias = alias.(gen.Alias)
		if targetAlias.Node != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if targetAlias.Node == "" {
		t.Fatal("alias not created")
	}

	// Spawn 3 subscribers
	sub1, err := node1.Spawn(factory_t8monitorAliasSub, gen.ProcessOptions{}, targetAlias)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8monitorAliasSub, gen.ProcessOptions{}, targetAlias)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8monitorAliasSub, gen.ProcessOptions{}, targetAlias)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify all monitoring
	info1, _ := node1.ProcessInfo(sub1)
	if len(info1.MonitorsAlias) != 1 {
		t.Fatal("sub1 should be monitoring")
	}

	// P1 demonitors
	node1.Send(sub1, "demonitor")
	time.Sleep(100 * time.Millisecond)

	info1, _ = node1.ProcessInfo(sub1)
	if len(info1.MonitorsAlias) != 0 {
		t.Error("sub1 should not be monitoring after demonitor")
	}

	// P2 demonitors
	node1.Send(sub2, "demonitor")
	time.Sleep(100 * time.Millisecond)

	// P3 demonitors (last)
	node1.Send(sub3, "demonitor")
	time.Sleep(100 * time.Millisecond)

	// Terminate target - nobody should get down
	node2.SendExit(targetPID, fmt.Errorf("test"))
	time.Sleep(200 * time.Millisecond)

	// All subs still alive
	if _, err := node1.ProcessInfo(sub1); err != nil {
		t.Error("sub1 should still be alive")
	}
	if _, err := node1.ProcessInfo(sub2); err != nil {
		t.Error("sub2 should still be alive")
	}
	if _, err := node1.ProcessInfo(sub3); err != nil {
		t.Error("sub3 should still be alive")
	}
}

// =============================================================================
// Test 2.2d: Partial demonitor for Event
// =============================================================================

func TestT8PartialDemonitorEvent(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "test123"
	opts1.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("n1de@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "test123"
	opts2.Log.DefaultLogger.Disable = true

	node2, err := ergo.StartNode("n2de@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// Spawn event producer
	producerPID, err := node2.Spawn(factory_t8eventProducer, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	// Get event
	var targetEvent gen.Event
	for i := 0; i < 10; i++ {
		event, err := node1.Call(producerPID, "getevent")
		if err != nil {
			t.Fatal(err)
		}
		targetEvent = event.(gen.Event)
		if targetEvent.Node != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if targetEvent.Node == "" {
		t.Fatal("event not created")
	}

	// Spawn 3 subscribers monitoring event
	sub1, err := node1.Spawn(factory_t8monitorEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	sub2, err := node1.Spawn(factory_t8monitorEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	sub3, err := node1.Spawn(factory_t8monitorEventSub, gen.ProcessOptions{}, targetEvent)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify all monitoring
	info1, err := node1.ProcessInfo(sub1)
	if err != nil {
		t.Fatalf("failed to get info for sub1: %v", err)
	}
	if len(info1.MonitorsEvent) != 1 {
		t.Fatal("sub1 should be monitoring event")
	}

	// P1 demonitors
	node1.Send(sub1, "demonitor")
	time.Sleep(100 * time.Millisecond)

	info1, _ = node1.ProcessInfo(sub1)
	if len(info1.MonitorsEvent) != 0 {
		t.Error("sub1 should not be monitoring after demonitor")
	}

	// P2 demonitors
	node1.Send(sub2, "demonitor")
	time.Sleep(100 * time.Millisecond)

	// P3 demonitors (last)
	node1.Send(sub3, "demonitor")
	time.Sleep(100 * time.Millisecond)

	// Terminate producer - nobody should get down
	node2.SendExit(producerPID, fmt.Errorf("test"))
	time.Sleep(200 * time.Millisecond)

	// All subs still alive
	if _, err := node1.ProcessInfo(sub1); err != nil {
		t.Error("sub1 should still be alive")
	}
	if _, err := node1.ProcessInfo(sub2); err != nil {
		t.Error("sub2 should still be alive")
	}
	if _, err := node1.ProcessInfo(sub3); err != nil {
		t.Error("sub3 should still be alive")
	}
}

// Helper: monitor event subscriber that can demonitor
func factory_t8demonitorEventSub() gen.ProcessBehavior {
	return &t8demonitorEventSub{}
}

type t8demonitorEventSub struct {
	act.Actor
	target gen.Event
}

func (s *t8demonitorEventSub) Init(args ...any) error {
	s.target = args[0].(gen.Event)
	s.Send(s.PID(), "domonitor")
	return nil
}

func (s *t8demonitorEventSub) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "domonitor" {
			_, err := s.MonitorEvent(s.target)
			return err
		}
		if m == "demonitor" {
			return s.DemonitorEvent(s.target)
		}
	case gen.MessageDownEvent:
		return gen.TerminateReasonNormal
	}
	return nil
}
