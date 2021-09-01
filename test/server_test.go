package test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
)

// This test is checking the cases below:
//
// initiation:
// - starting 2 nodes (node1, node2)
// - starting 4 Servers
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

type testServer struct {
	gen.Server
	res chan interface{}
}

func (tgs *testServer) Init(process *gen.ServerProcess, args ...etf.Term) error {
	tgs.res <- nil
	return nil
}
func (tgs *testServer) HandleCast(process *gen.ServerProcess, message etf.Term) string {
	tgs.res <- message
	return "noreply"
}
func (tgs *testServer) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (string, etf.Term) {
	return "reply", message
}
func (tgs *testServer) HandleInfo(process *gen.ServerProcess, message etf.Term) string {
	tgs.res <- message
	return "noreply"
}
func (tgs *testServer) Terminate(process *gen.ServerProcess, reason string) {
	tgs.res <- reason
}

type testServerDirect struct {
	gen.Server
	err chan error
}

func (tgsd *testServerDirect) Init(process *gen.ServerProcess, args ...etf.Term) error {
	tgsd.err <- nil
	return nil
}
func (tgsd *testServerDirect) HandleDirect(process *gen.ServerProcess, message interface{}) (interface{}, error) {
	return message, nil
}

func TestServer(t *testing.T) {
	fmt.Printf("\n=== Test Server\n")
	fmt.Printf("Starting nodes: nodeGS1@localhost, nodeGS2@localhost: ")
	node1, _ := ergo.StartNode("nodeGS1@localhost", "cookies", node.Options{})
	node2, _ := ergo.StartNode("nodeGS2@localhost", "cookies", node.Options{})
	if node1 == nil || node2 == nil {
		t.Fatal("can't start nodes")
	} else {
		fmt.Println("OK")
	}

	gs1 := &testServer{
		res: make(chan interface{}, 2),
	}
	gs2 := &testServer{
		res: make(chan interface{}, 2),
	}
	gs3 := &testServer{
		res: make(chan interface{}, 2),
	}
	gsDirect := &testServerDirect{
		err: make(chan error, 2),
	}

	fmt.Printf("    wait for start of gs1 on %#v: ", node1.NodeName())
	node1gs1, _ := node1.Spawn("gs1", gen.ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.res, nil)

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.NodeName())
	node1gs2, _ := node1.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.res, nil)

	fmt.Printf("    wait for start of gs3 on %#v: ", node2.NodeName())
	node2gs3, _ := node2.Spawn("gs3", gen.ProcessOptions{}, gs3, nil)
	waitForResultWithValue(t, gs3.res, nil)

	fmt.Printf("    wait for start of gsDirect on %#v: ", node2.NodeName())
	node2gsDirect, _ := node2.Spawn("gsDirect", gen.ProcessOptions{}, gsDirect, nil)
	waitForResult(t, gsDirect.err)

	fmt.Println("Testing Server process:")

	fmt.Printf("    process.Send (by Pid) local (gs1) -> local (gs2) : ")
	node1gs1.Send(node1gs2.Self(), etf.Atom("hi"))
	waitForResultWithValue(t, gs2.res, etf.Atom("hi"))

	node1gs1.Cast(node1gs2.Self(), etf.Atom("hi cast"))
	fmt.Printf("    process.Cast (by Pid) local (gs1) -> local (gs2) : ")
	waitForResultWithValue(t, gs2.res, etf.Atom("hi cast"))

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
	waitForResultWithValue(t, gs2.res, etf.Atom("hi"))

	node1gs1.Cast(etf.Atom("gs2"), etf.Atom("hi cast"))
	fmt.Printf("    process.Cast (by Name) local (gs1) -> local (gs2) : ")
	waitForResultWithValue(t, gs2.res, etf.Atom("hi cast"))

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
	waitForResultWithValue(t, gs2.res, etf.Atom("hi"))

	node1gs1.Cast(alias, etf.Atom("hi cast"))
	fmt.Printf("    process.Cast (by Alias) local (gs1) -> local (gs2) : ")
	waitForResultWithValue(t, gs2.res, etf.Atom("hi cast"))

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
	waitForResultWithValue(t, gs3.res, etf.Atom("hi"))

	node1gs1.Cast(node2gs3.Self(), etf.Atom("hi cast"))
	fmt.Printf("    process.Cast (by Pid) local (gs1) -> remote (gs3) : ")
	waitForResultWithValue(t, gs3.res, etf.Atom("hi cast"))

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
	processName := etf.Tuple{"gs3", node2.NodeName()}
	node1gs1.Send(processName, etf.Atom("hi"))
	waitForResultWithValue(t, gs3.res, etf.Atom("hi"))

	node1gs1.Cast(processName, etf.Atom("hi cast"))
	fmt.Printf("    process.Cast (by Name) local (gs1) -> remote (gs3) : ")
	waitForResultWithValue(t, gs3.res, etf.Atom("hi cast"))

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
	waitForResultWithValue(t, gs3.res, etf.Atom("hi"))

	node1gs1.Cast(alias, etf.Atom("hi cast"))
	fmt.Printf("    process.Cast (by Alias) local (gs1) -> remote (gs3) : ")
	waitForResultWithValue(t, gs3.res, etf.Atom("hi cast"))

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

	fmt.Printf("    process.SetTrapExit(true) and call process.Exit() gs2: ")
	node1gs2.SetTrapExit(true)
	node1gs2.Exit("test trap")
	waitForResultWithValue(t, gs2.res, gen.MessageExit{node1gs2.Self(), "test trap"})
	fmt.Printf("    check process.IsAlive gs2 (must be alive): ")
	if !node1gs2.IsAlive() {
		t.Fatal("should be alive")
	}
	fmt.Println("OK")

	fmt.Printf("    process.SetTrapExit(false) and call process.Exit() gs2: ")
	node1gs2.SetTrapExit(false)
	node1gs2.Exit("test trap")
	waitForResultWithValue(t, gs2.res, "test trap")

	fmt.Printf("    check process.IsAlive gs2 (must be died): ")
	if node1gs2.IsAlive() {
		t.Fatal("shouldn't be alive")
	}
	fmt.Println("OK")

	fmt.Printf("Stopping nodes: %v, %v\n", node1.NodeName(), node2.NodeName())
	node1.Stop()
	node2.Stop()
}

func waitForResult(t *testing.T, w chan error) {
	select {
	case e := <-w:
		if e == nil {
			fmt.Println("OK")
			return
		}

		t.Fatal(e)

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
