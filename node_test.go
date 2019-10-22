package ergonode

import (
	"fmt"
	"net"
	"testing"
)

func TestNode(t *testing.T) {
	opts := NodeOptions{
		ListenRangeBegin: 25001,
		ListenRangeEnd:   25001,
		EPMDPort:         24999,
	}

	node := CreateNode("node@localhost", "cookies", opts)

	if conn, err := net.Dial("tcp", ":25001"); err != nil {
		fmt.Println("Connect to the node' listening port FAILED")
		t.Fatal(err)
	} else {
		defer conn.Close()
	}

	if conn, err := net.Dial("tcp", ":24999"); err != nil {
		fmt.Println("Connect to the node' listening EPMD port FAILED")
		t.Fatal(err)
	} else {
		defer conn.Close()
	}

	p, e := node.Spawn("", ProcessOptions{}, &GenServer{})

	if e != nil {
		t.Fatal(e)
	}
	// empty GenServer{} should gonna die immediately
	if node.IsProcessAlive(p.Self()) {
		t.Fatal("IsProcessAlive: expect 'false', but got 'true'")
	}

	gs1 := &testGenServer{
		err: make(chan error, 2),
	}
	p, e = node.Spawn("", ProcessOptions{}, gs1)
	if e != nil {
		t.Fatal(e)
	}

	if !node.IsProcessAlive(p.Self()) {
		t.Fatal("IsProcessAlive: expect 'true', but got 'false'")
	}

	_, ee := node.ProcessInfo(p.Self())
	if ee != nil {
		t.Fatal(ee)
	}

	node.Stop()
}
