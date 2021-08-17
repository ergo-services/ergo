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

func (tgs *testGenServer) Init(state *GenServerState, args ...etf.Term) error {
	tgs.err <- nil
	return nil
}
func (tgs *testGenServer) HandleCast(state *GenServerState, message etf.Term) string {
	// fmt.Printf("testGenServer ({%s, %s}): HandleCast: %#v\n", tgs.process.name, tgs.process.Node.Name(), message)
	tgs.err <- nil
	return "noreply"
}
func (tgs *testGenServer) HandleCall(state *GenServerState, from GenServerFrom, message etf.Term) (string, etf.Term) {
	// fmt.Printf("testGenServer ({%s, %s}): HandleCall: %#v, From: %#v\n", tgs.process.name, tgs.process.Node.Name(), message, from)
	return "reply", message
}
func (tgs *testGenServer) HandleInfo(state *GenServerState, message etf.Term) string {
	// fmt.Printf("testGenServer ({%s, %s}): HandleInfo: %#v\n", tgs.process.name, tgs.process.Node.Name(), message)
	tgs.err <- nil
	return "noreply"
}
func (tgs *testGenServer) Terminate(state *GenServerState, reason string) {
	// fmt.Printf("testGenServer ({%s, %s}): Terminate: %#v\n", tgs.process.name, tgs.process.Node.Name(), reason)
	tgs.err <- nil
}

type testGenServerDirect struct {
	GenServer
	err chan error
}

func (tgsd *testGenServerDirect) Init(state *GenServerState, args ...etf.Term) error {
	tgsd.err <- nil
	return nil
}
func (tgsd *testGenServerDirect) HandleDirect(state *GenServerState, message interface{}) (interface{}, error) {
	return message, nil
}

func TestGenServer(t *testing.T) {
	fmt.Printf("\n=== Test GenServer\n")
	fmt.Printf("Starting nodes: nodeGS1@localhost, nodeGS2@localhost: ")
	node1, _ := CreateNode("nodeGS1@localhost", "cookies", NodeOptions{})
	node2, _ := CreateNode("nodeGS2@localhost", "cookies", NodeOptions{})
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
	gsDirect := &testGenServerDirect{
		err: make(chan error, 2),
	}

	fmt.Printf("    wait for start of gs1 on %#v: ", node1.Name())
	node1gs1, _ := node1.Spawn("gs1", ProcessOptions{}, gs1, nil)
	waitForResult(t, gs1.err)

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.Name())
	node1gs2, _ := node1.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResult(t, gs2.err)

	fmt.Printf("    wait for start of gs3 on %#v: ", node2.Name())
	node2gs3, _ := node2.Spawn("gs3", ProcessOptions{}, gs3, nil)
	waitForResult(t, gs3.err)

	fmt.Printf("    wait for start of gsDirect on %#v: ", node2.Name())
	node2gsDirect, _ := node2.Spawn("gsDirect", ProcessOptions{}, gsDirect, nil)
	waitForResult(t, gsDirect.err)

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
	alias, err := node1gs2.CreateAlias()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("    process.Send (by Alias) local (gs1) -> local (gs2) : ")
	node1gs1.Send(alias, etf.Atom("hi"))
	waitForResult(t, gs2.err)

	node1gs1.Cast(alias, etf.Atom("hi cast"))
	fmt.Printf("    process.Cast (by Alias) local (gs1) -> local (gs2) : ")
	waitForResult(t, gs2.err)

	fmt.Printf("    process.Call (by Alias) local (gs1) -> local (gs2): ")
	if v1, err := node1gs1.Call(alias, v); err != nil {
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
	processName := etf.Tuple{"gs3", node2.Name()}
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

	fmt.Printf("    process.Send (by Alias) local (gs1) -> remote (gs3) : ")
	alias, err = node2gs3.CreateAlias()
	if err != nil {
		t.Fatal(err)
	}

	node1gs1.Send(alias, etf.Atom("hi"))
	waitForResult(t, gs3.err)

	node1gs1.Cast(alias, etf.Atom("hi cast"))
	fmt.Printf("    process.Cast (by Alias) local (gs1) -> remote (gs3) : ")
	waitForResult(t, gs3.err)

	fmt.Printf("    process.Call (by Alias) local (gs1) -> remote (gs3): ")
	if v1, err := node1gs1.Call(alias, v); err != nil {
		t.Fatal(err)
	} else {
		if v == v1 {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("expected: %#v , got: %#v", v, v1)
			t.Fatal(e)
		}
	}

	fmt.Printf("    process.Direct (without HandleDirect implementation): ")
	if _, err := node1gs1.Direct(nil); err == nil {
		t.Fatal("must be ErrUnsupportedRequest")
	} else {
		fmt.Println("OK")
	}
	fmt.Printf("    process.Direct (with HandleDirect implementation): ")
	if v1, err := node2gsDirect.Direct(v); err != nil {
		t.Fatal(err)
	} else {
		if v == v1 {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("expected: %#v , got: %#v", v, v1)
			t.Fatal(e)
		}
	}

	fmt.Printf("Stopping nodes: %v, %v\n", node1.Name(), node2.Name())
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

func waitForResultWithMultiValue(t *testing.T, w chan interface{}, values etf.List) {

	select {
	case v := <-w:
		found := false
		i := 0
		for {
			if reflect.DeepEqual(v, values[i]) {
				found = true
				values[i] = values[0]
				values = values[1:]
				if len(values) == 0 {
					return
				}
				// i dont care about stack growing since 'values'
				// usually short
				waitForResultWithMultiValue(t, w, values)
				break
			}
			i++
			if i+1 > len(values) {
				break
			}
		}

		if !found {
			e := fmt.Errorf("got unexpected value: %#v", v)
			t.Fatal(e)
		}

	case <-time.After(time.Second * time.Duration(2)):
		t.Fatal("result timeout")
	}
	fmt.Println("OK")
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

func waitForTimeout(t *testing.T, w chan interface{}) {
	select {
	case v := <-w:
		e := fmt.Errorf("got value we shouldn't receive: %#v", v)
		t.Fatal(e)

	case <-time.After(time.Millisecond * time.Duration(300)):
		return
	}
}
