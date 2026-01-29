package distributed

import (
	"fmt"
	"testing"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type t8monitor struct {
	act.Actor
	downChan      chan gen.MessageDownPID
	downAliasChan chan gen.MessageDownAlias
}

func (t *t8monitor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch m := request.(type) {
	case gen.PID:
		// Call remote process to get its alias
		res, err := t.Call(m, "get_alias")
		if err != nil {
			return nil, err
		}
		alias := res.(gen.Alias)
		// Monitor both
		if err := t.MonitorPID(m); err != nil {
			return nil, err
		}
		if err := t.MonitorAlias(alias); err != nil {
			return nil, err
		}
		return alias, nil
	}
	return nil, nil
}

func (t *t8monitor) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case gen.MessageDownPID:
		t.downChan <- m
	case gen.MessageDownAlias:
		t.downAliasChan <- m
	}
	return nil
}

type t8target struct {
	act.Actor
}

func (t *t8target) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request.(type) {
	case string:
		return t.CreateAlias()
	}
	return nil, nil
}

func TestT8MonitorIncarnationFix(t *testing.T) {
	// 1. Start Node1
	options1 := gen.NodeOptions{}
	options1.Network.Cookie = "fix_test_cookie"
	options1.Log.DefaultLogger.Disable = true
	node1, err := ergo.StartNode("node1@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	// 2. Wait for 2 seconds to ensure different creation time
	time.Sleep(2 * time.Second)

	// 3. Start Node2
	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "fix_test_cookie"
	options2.Log.DefaultLogger.Disable = true
	node2, err := ergo.StartNode("node2@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	// 4. Connect node1 to node2
	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	// 5. Spawn monitor process on node1
	downChan := make(chan gen.MessageDownPID, 1)
	downAliasChan := make(chan gen.MessageDownAlias, 1)
	pidMonitor, err := node1.Spawn(func() gen.ProcessBehavior {
		return &t8monitor{downChan: downChan, downAliasChan: downAliasChan}
	}, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// 6. Spawn target process on node2
	pidTarget, err := node2.Spawn(func() gen.ProcessBehavior {
		return &t8target{}
	}, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// 7. Request monitor to setup monitoring
	errChan := make(chan error, 1)
	node1.Spawn(func() gen.ProcessBehavior {
		return &t8runner{monitor: pidMonitor, target: pidTarget, errChan: errChan}
	}, gen.ProcessOptions{})

	if err := <-errChan; err != nil {
		t.Fatal(err)
	}

	// Verify creation times are different
	fmt.Printf("Node1 creation: %d, Node2 creation: %d\n", node1.Creation(), node2.Creation())

	// 8. Stop target process on node2
	node2.Kill(pidTarget)

	// 9. Wait for MessageDownPID on node1
	select {
	case down := <-downChan:
		fmt.Printf("Successfully received MessageDownPID for %s\n", down.PID)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for MessageDownPID")
	}

	// 10. Wait for MessageDownAlias on node1
	select {
	case down := <-downAliasChan:
		fmt.Printf("Successfully received MessageDownAlias for %s\n", down.Alias)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for MessageDownAlias")
	}
}

type t8runner struct {
	act.Actor
	monitor gen.PID
	target  gen.PID
	errChan chan error
}

func (t *t8runner) Init(args ...any) error {
	return nil
}

func (t *t8runner) ProcessRun() error {
	_, err := t.Call(t.monitor, t.target)
	t.errChan <- err
	return gen.TerminateReasonNormal
}
