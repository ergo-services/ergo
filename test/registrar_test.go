package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
)

type TestRegistrarGenserver struct {
	gen.Server
}

func (trg *TestRegistrarGenserver) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (string, etf.Term) {
	// fmt.Printf("TestRegistrarGenserver ({%s, %s}): HandleCall: %#v, From: %#v\n", trg.process.name, trg.process.Node.Name(), message, from)
	return "reply", message
}

func TestRegistrar(t *testing.T) {
	fmt.Printf("\n=== Test Registrar\n")
	fmt.Printf("Starting nodes: nodeR1@localhost, nodeR2@localhost: ")
	node1, _ := ergo.StartNode("nodeR1@localhost", "cookies", node.Options{})
	node2, _ := ergo.StartNode("nodeR2@localhost", "cookies", node.Options{})
	defer node1.Stop()
	defer node2.Stop()
	if node1 == nil || node2 == nil {
		t.Fatal("can't start nodes")
	} else {
		fmt.Println("OK")
	}

	gs := &TestRegistrarGenserver{}
	fmt.Printf("Starting TestRegistrarGenserver. registering as 'gs1' on %s and create an alias: ", node1.Name())
	node1gs1, err := node1.Spawn("gs1", gen.ProcessOptions{}, gs, nil)
	if err != nil {
		t.Fatal(err)
	}
	alias, err := node1gs1.CreateAlias()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("...get process by name 'gs1': ")
	p := node1.ProcessByName("gs1")
	if p == nil {
		message := fmt.Sprintf("missing process %v on %s", node1gs1.Self(), node1.Name())
		t.Fatal(message)
	}
	fmt.Println("OK")
	fmt.Printf("...get process by pid of 'gs1': ")
	p1 := node1.ProcessByPid(node1gs1.Self())
	if p1 == nil {
		message := fmt.Sprintf("missing process %v on %s", node1gs1.Self(), node1.Name())
		t.Fatal(message)
	}

	if p != p1 {
		message := fmt.Sprintf("not equal: %v on %s", p.Self(), p1.Self())
		t.Fatal(message)
	}
	fmt.Println("OK")

	fmt.Printf("...get process by alias of 'gs1': ")
	p2 := node1.ProcessByAlias(alias)
	if p2 == nil {
		message := fmt.Sprintf("missing process %v on %s", node1gs1.Self(), node1.Name())
		t.Fatal(message)
	}

	if p1 != p2 {
		message := fmt.Sprintf("not equal: %v on %s", p1.Self(), p2.Self())
		t.Fatal(message)
	}
	fmt.Println("OK")

	fmt.Printf("...registering name 'test' related to %v: ", node1gs1.Self())
	if e := node1.Register("test", node1gs1.Self()); e != nil {
		t.Fatal(e)
	} else {
		if e := node1.Register("test", node1gs1.Self()); e == nil {
			t.Fatal("registered duplicate name")
		}
	}
	fmt.Println("OK")
	fmt.Printf("...unregistering name 'test' related to %v: ", node1gs1.Self())
	node1.Unregister("test")
	if e := node1.Register("test", node1gs1.Self()); e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting TestRegistrarGenserver and registering as 'gs2' on %s: ", node2.Name())
	node2gs2, _ := node2.Spawn("gs2", ProcessOptions{}, gs, nil)
	if _, ok := node2.registrar.processes[node2gs2.Self().ID]; !ok {
		message := fmt.Sprintf("missing process %v on %s", node2gs2.Self(), node2.Name())
		t.Fatal(message)
	}
	fmt.Println("OK")
}

func TestRegistrarAlias(t *testing.T) {
	fmt.Printf("\n=== Test Registrar Alias\n")
	fmt.Printf("Starting node: nodeR1Alias@localhost: ")
	node1, _ := CreateNode("nodeR1Alias@localhost", "cookies", NodeOptions{})
	defer node1.Stop()
	if node1 == nil {
		t.Fatal("can't start nodes")
	} else {
		fmt.Println("OK")
	}

	gs := &TestRegistrarGenserver{}
	fmt.Printf("    Starting gs1 and gs2 GenServers on %s: ", node1.Name())
	node1gs1, err := node1.Spawn("gs1", ProcessOptions{}, gs, nil)
	if err != nil {
		t.Fatal(err)
	}
	node1gs2, err := node1.Spawn("gs2", ProcessOptions{}, gs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(node1.registrar.aliases) > 0 {
		t.Fatal("alias table must be empty")
	}

	fmt.Println("OK")

	fmt.Printf("    Create gs1 alias: ")
	alias, err := node1gs1.CreateAlias()
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := node1.registrar.aliases[alias]; !ok {
		if p.self != p.self {
			t.Fatal("wrong alias")
		}
		t.Fatal("missing alias")
	}
	fmt.Println("OK")
	fmt.Printf("    Make a call to gs1 via alias: ")
	if reply, err := node1gs2.Call(alias, "hi"); err == nil {
		if r, ok := reply.(string); !ok || r != "hi" {
			t.Fatal("wrong result", reply)
		}
	} else {
		t.Fatal(err)
	}
	fmt.Println("OK")
	fmt.Printf("    Delete gs1 alias by gs2 (shouldn't be allowed): ")
	if err := node1gs2.DeleteAlias(alias); err != ErrAliasOwner {
		t.Fatal(" expected ErrAliasOwner, got:", err)
	}
	fmt.Println("OK")
	fmt.Printf("    Delete gs1 alias by itself: ")
	if err := node1gs1.DeleteAlias(alias); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	if len(node1.registrar.aliases) > 0 {
		t.Fatal("alias table (registrar) must be empty", node1.registrar.aliases)
	}

	if len(node1gs1.aliases) > 0 {
		t.Fatal("alias table (process) must be empty", node1gs1.aliases)

	}
	fmt.Printf("    Aliases must be cleaned up once the owner is down: ")
	node1gs1.CreateAlias()
	node1gs1.CreateAlias()
	node1gs1.CreateAlias()
	if len(node1.registrar.aliases) != 3 {
		t.Fatal("alias table (registrar) must have 3 aliases", node1.registrar.aliases)
	}

	if len(node1gs1.aliases) != 3 {
		t.Fatal("alias table (process) must have 3 aliases", node1gs1.aliases)
	}
	node1gs1.Kill()
	time.Sleep(100 * time.Millisecond)
	if len(node1.registrar.aliases) != 0 {
		t.Fatal("alias table (registrar) must be empty", node1.registrar.aliases)
	}
	fmt.Println("OK")

	fmt.Printf("    Create gs1 alias on a stopped process (shouldn't be allowed): ")
	alias, err = node1gs1.CreateAlias()
	if err != ErrProcessUnknown {
		t.Fatal("wrong result")
	}
	fmt.Println("OK")

}
