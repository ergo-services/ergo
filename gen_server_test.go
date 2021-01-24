package ergo

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/halturin/ergo/etf"
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

type testGenServer struct {
	GenServer
	err chan error
}

func (tgs *testGenServer) Init(p *Process, args ...interface{}) (state interface{}) {
	tgs.err <- nil
	return nil
}
func (tgs *testGenServer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	// fmt.Printf("testGenServer ({%s, %s}): HandleCast: %#v\n", tgs.process.name, tgs.process.Node.FullName, message)
	tgs.err <- nil
	return "noreply", state
}
func (tgs *testGenServer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	// fmt.Printf("testGenServer ({%s, %s}): HandleCall: %#v, From: %#v\n", tgs.process.name, tgs.process.Node.FullName, message, from)
	return "reply", message, state
}
func (tgs *testGenServer) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	// fmt.Printf("testGenServer ({%s, %s}): HandleInfo: %#v\n", tgs.process.name, tgs.process.Node.FullName, message)
	tgs.err <- nil
	return "noreply", state
}
func (tgs *testGenServer) Terminate(reason string, state interface{}) {
	// fmt.Printf("testGenServer ({%s, %s}): Terminate: %#v\n", tgs.process.name, tgs.process.Node.FullName, reason)
	tgs.err <- nil
}

func TestGenServer(t *testing.T) {
	fmt.Printf("\n=== Test GenServer\n")
	fmt.Printf("Starting nodes: nodeGS1@localhost, nodeGS2@localhost: ")
	node1 := CreateNode("nodeGS1@localhost", "cookies", NodeOptions{})
	node2 := CreateNode("nodeGS2@localhost", "cookies", NodeOptions{})
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

	fmt.Println("Testing GenServer process:")

	fmt.Printf("    process.Send (by Pid) local (gs1) -> local (gs2) : ")
	node1gs1.Send(node1gs2.Self(), etf.Atom("hi"))
	waitForResult(t, gs2.err)

	node1gs1.Cast(node1gs2.Self(), etf.Atom("hi cast"))
	fmt.Printf("    process.Cast (by Pid) local (gs1) -> local (gs2) : ")
	waitForResult(t, gs2.err)

	fmt.Printf("    process.Call (by Pid) local (gs1) -> local (gs2): ")
	v := etf.Atom("hi call")
	if v1, err := node1gs1.Call(node1gs2.Self(), v); err != nil {
		t.Fatal(err)
	} else {
		if v == v1 {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("expected: %#v , got: %#v", v, v1)
			t.Fatal(e)
		}
	}

	fmt.Printf("    process.Send (by Name) local (gs1) -> local (gs2) : ")
	node1gs1.Send(etf.Atom("gs2"), etf.Atom("hi"))
	waitForResult(t, gs2.err)

	node1gs1.Cast(etf.Atom("gs2"), etf.Atom("hi cast"))
	fmt.Printf("    process.Cast (by Name) local (gs1) -> local (gs2) : ")
	waitForResult(t, gs2.err)

	fmt.Printf("    process.Call (by Name) local (gs1) -> local (gs2): ")
	if v1, err := node1gs1.Call(etf.Atom("gs2"), v); err != nil {
		t.Fatal(err)
	} else {
		if v == v1 {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("expected: %#v , got: %#v", v, v1)
			t.Fatal(e)
		}
	}

	fmt.Printf("    process.Send (by Pid) local (gs1) -> remote (gs3) : ")
	node1gs1.Send(node2gs3.Self(), etf.Atom("hi"))
	waitForResult(t, gs3.err)

	node1gs1.Cast(node2gs3.Self(), etf.Atom("hi cast"))
	fmt.Printf("    process.Cast (by Pid) local (gs1) -> remote (gs3) : ")
	waitForResult(t, gs3.err)

	fmt.Printf("    process.Call (by Pid) local (gs1) -> remote (gs3): ")
	if v1, err := node1gs1.Call(node2gs3.Self(), v); err != nil {
		t.Fatal(err)
	} else {
		if v == v1 {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("expected: %#v , got: %#v", v, v1)
			t.Fatal(e)
		}
	}

	fmt.Printf("    process.Send (by Name) local (gs1) -> remote (gs3) : ")
	processName := etf.Tuple{"gs3", node2.FullName}
	node1gs1.Send(processName, etf.Atom("hi"))
	waitForResult(t, gs3.err)

	node1gs1.Cast(processName, etf.Atom("hi cast"))
	fmt.Printf("    process.Cast (by Name) local (gs1) -> remote (gs3) : ")
	waitForResult(t, gs3.err)

	fmt.Printf("    process.Call (by Name) local (gs1) -> remote (gs3): ")
	if v1, err := node1gs1.Call(processName, v); err != nil {
		t.Fatal(err)
	} else {
		if v == v1 {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("expected: %#v , got: %#v", v, v1)
			t.Fatal(e)
		}
	}

	fmt.Printf("Stopping nodes: %v, %v\n", node1.FullName, node2.FullName)
	node1.Stop()
	node2.Stop()
}

func waitForResult(t *testing.T, w chan error) {
	select {
	case e := <-w:
		if e == nil {
			fmt.Println("OK")
		}

	case <-time.After(time.Second * time.Duration(1)):
		t.Fatal("result timeout")
	}
}

func waitForResultWithValue(t *testing.T, w chan interface{}, value interface{}) {
	select {
	case v := <-w:
		if reflect.DeepEqual(v, value) {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("expected: %#v , got: %#v", value, v)
			t.Fatal(e)
		}

	case <-time.After(time.Second * time.Duration(2)):
		t.Fatal("result timeout")
	}
}

func waitForResultWithValueOrValue(t *testing.T, w chan interface{}, value1, value2 interface{}) {
	select {
	case v := <-w:
		if reflect.DeepEqual(v, value1) {
			fmt.Println("OK")
		} else {
			if reflect.DeepEqual(v, value2) {
				fmt.Println("OK")
			} else {
				e := fmt.Errorf("expected another value, but got: %#v", v)
				t.Fatal(e)
			}
		}

	case <-time.After(time.Second * time.Duration(2)):
		t.Fatal("result timeout")
	}
}

func waitForTimeout(w chan interface{}) error {
	select {
	case v := <-w:
		return fmt.Errorf("got value we shouldn't receive: %#v", v)

	case <-time.After(time.Millisecond * time.Duration(300)):
		return nil
	}
}
