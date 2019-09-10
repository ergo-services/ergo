package ergonode

import (
	"fmt"
	"testing"
)

// This test is checking the cases below:
//
// initiation:
// - starting 2 nodes (node1, node2)
// - starting 4 GenServers
//	 * 2 on node1 - gs1, gs2
// 	 * 2 on node2 - gs3, gs4
//
// checking:
// - local sending
//  * send: node1 (gs1) -> node1 (gs2). in fashion of erlang sending `erlang:send`
//  * cast: node1 (gs1) -> node1 (gs2). like `gen_server:cast` does
//  * call: node1 (gs1) -> node1 (gs2). like `gen_server:call` does
//
// - remote sending
//  * send: node1 (gs1) -> node2 (gs3)
//  * cast: node1 (gs1) -> node2 (gs3)
//  * call: node1 (gs1) -> node2 (gs3)

func TestMonitor(t *testing.T) {
	fmt.Printf("\n== Test Monitor/Link\n")
	fmt.Printf("Starting nodes: nodeM1@localhost, nodeM2@localhost: ")
	node1 := CreateNode("nodeM1@localhost", "cookies", NodeOptions{})
	node2 := CreateNode("nodeM2@localhost", "cookies", NodeOptions{})
	if node1 == nil || node2 == nil {
		t.Fatal("can't start nodes")
	} else {
		fmt.Println("OK")
	}

	gs1 := &testGenServer{
		err: make(chan error, 2),
	}
	gs2 := &testGenServer{
		err: make(chan error, 2),
	}
	gs3 := &testGenServer{
		err: make(chan error, 2),
	}

	fmt.Printf("    wait for start of gs1 on %#v: ", node1.FullName)
	node1gs1, _ := node1.Spawn("gs1", ProcessOptions{}, gs1, nil)
	waitForResult(t, gs1.err)

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.FullName)
	node1gs2, _ := node1.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResult(t, gs2.err)

	fmt.Printf("    wait for start of gs3 on %#v: ", node2.FullName)
	node2gs3, _ := node2.Spawn("gs3", ProcessOptions{}, gs3, nil)
	waitForResult(t, gs3.err)

	fmt.Println("Testing Monitor process:")

	fmt.Printf("aaaa %v %v %v", node1gs1, node1gs2, node2gs3)

	fmt.Printf("Stopping nodes: %v, %v\n", node1.FullName, node2.FullName)
	node1.Stop()
	node2.Stop()

	fmt.Printf("    waiting for termination of gs1: ")
	waitForResult(t, gs1.err)
	fmt.Printf("    waiting for termination of gs2: ")
	waitForResult(t, gs2.err)
	fmt.Printf("    waiting for termination of gs3: ")
	waitForResult(t, gs3.err)

}
