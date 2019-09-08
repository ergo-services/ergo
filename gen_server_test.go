package ergonode

import (
	"fmt"
	"testing"

	"github.com/halturin/ergonode/etf"
)

type testGenServer struct {
	GenServer
	process Process
	name    string
}

func (tgs *testGenServer) Init(p Process, args ...interface{}) (state interface{}) {
	tgs.process = p
	return nil
}
func (tgs *testGenServer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("testGenServer (%s): HandleCast: %#v\n", tgs.name, message)
	return "noreply", state
}
func (tgs *testGenServer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	fmt.Printf("testGenServer (%s): HandleCall: %#v, From: %#v\n", tgs.name, message, from)
	reply := etf.Term(etf.Atom("reply"))
	return "reply", reply, state
}
func (tgs *testGenServer) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("testGenServer (%s): HandleInfo: %#v\n", tgs.name, message)
	return "noreply", state
}
func (tgs *testGenServer) Terminate(reason string, state interface{}) {
	fmt.Printf("testGenServer (%s): Terminate: %#v\n", tgs.name, reason)
}

func TestGenServer(t *testing.T) {
	node := CreateNode("node@localhost", "cookies", NodeOptions{})

	gs1 := &testGenServer{name: "gs1"}
	gs2 := &testGenServer{name: "gs2"}

	p1, _ := node.Spawn("gs1", ProcessOptions{}, gs1, nil)
	p2, _ := node.Spawn("gs2", ProcessOptions{}, gs2, nil)

	p1.Send(p2.Self(), etf.Atom("hi"))
	p1.Cast(p2.Self(), etf.Atom("hi cast"))
	if _, err := p1.Call(p2.Self(), etf.Atom("hi call")); err != nil {
		t.Fatal(err)
	}

	p1.Stop("normal")
	p2.Stop("shutdown")

	node.Stop()
}
