package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

type TestCoreGenserver struct {
	gen.Server
}

func (trg *TestCoreGenserver) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	// fmt.Printf("TestCoreGenserver ({%s, %s}): HandleCall: %#v, From: %#v\n", trg.process.name, trg.process.Node.Name(), message, from)
	return message, gen.ServerStatusOK
}

func (trg *TestCoreGenserver) HandleDirect(process *gen.ServerProcess, ref etf.Ref, message interface{}) (interface{}, gen.DirectStatus) {
	switch m := message.(type) {
	case makeCall:
		return process.Call(m.to, m.message)
	}
	return nil, gen.ErrUnsupportedRequest
}

func TestCore(t *testing.T) {
	fmt.Printf("\n=== Test Registrar\n")
	fmt.Printf("Starting nodes: nodeR1@localhost, nodeR2@localhost: ")
	node1, _ := ergo.StartNode("nodeR1@localhost", "cookies", node.Options{})
	defer node1.Stop()
	if node1 == nil {
		t.Fatal("can't start nodes")
	} else {
		fmt.Println("OK")
	}

	gs := &TestCoreGenserver{}
	fmt.Printf("Starting TestCoreGenserver. registering as 'gs1' on %s and create an alias: ", node1.Name())
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
	if e := node1.RegisterName("test", node1gs1.Self()); e != nil {
		t.Fatal(e)
	} else {
		if e := node1.RegisterName("test", node1gs1.Self()); e == nil {
			t.Fatal("registered duplicate name")
		}
	}
	fmt.Println("OK")
	fmt.Printf("...unregistering name 'test' related to %v: ", node1gs1.Self())
	node1.UnregisterName("test")
	if e := node1.RegisterName("test", node1gs1.Self()); e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting TestCoreGenserver and registering as 'gs2' on %s: ", node1.Name())
	node1gs2, err := node1.Spawn("gs2", gen.ProcessOptions{}, gs, nil)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("...try to unregister 'test' related to %v using gs2 process (not allowed): ", node1gs1.Self())
	if err := node1gs2.UnregisterName("test"); err != node.ErrNameOwner {
		t.Fatal("not allowed to unregister by not an owner")
	}
	fmt.Println("OK")

	fmt.Printf("...try to unregister 'test' related to %v using gs1 process (owner): ", node1gs1.Self())
	if err := node1gs1.UnregisterName("test"); err != nil {
		t.Fatal(err)
	}

	fmt.Println("OK")
}

func TestCoreAlias(t *testing.T) {
	fmt.Printf("\n=== Test Registrar Alias\n")
	fmt.Printf("Starting node: nodeR1Alias@localhost: ")
	node1, _ := ergo.StartNode("nodeR1Alias@localhost", "cookies", node.Options{})
	defer node1.Stop()
	if node1 == nil {
		t.Fatal("can't start nodes")
	} else {
		fmt.Println("OK")
	}

	gs := &TestCoreGenserver{}
	fmt.Printf("    Starting gs1 and gs2 GenServers on %s: ", node1.Name())
	node1gs1, err := node1.Spawn("gs1", gen.ProcessOptions{}, gs, nil)
	if err != nil {
		t.Fatal(err)
	}
	node1gs2, err := node1.Spawn("gs2", gen.ProcessOptions{}, gs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(node1gs1.Aliases()) > 0 || len(node1gs2.Aliases()) > 0 {
		t.Fatal("alias table must be empty")
	}

	fmt.Println("OK")

	fmt.Printf("    Create gs1 alias: ")
	alias, err := node1gs1.CreateAlias()
	if err != nil {
		t.Fatal(err)
	}
	prc := node1gs1.ProcessByAlias(alias)
	if prc == nil {
		t.Fatal("missing alias")
	}
	if prc.Self() != node1gs1.Self() {
		t.Fatal("wrong alias")
	}
	fmt.Println("OK")

	fmt.Printf("    Make a call to gs1 via alias: ")
	call := makeCall{
		to:      alias,
		message: "hi",
	}
	if reply, err := node1gs2.Direct(call); err == nil {
		if r, ok := reply.(string); !ok || r != "hi" {
			t.Fatal("wrong result", reply)
		}
	} else {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("    Delete gs1 alias by gs2 (not allowed): ")
	if err := node1gs2.DeleteAlias(alias); err != node.ErrAliasOwner {
		t.Fatal(" expected ErrAliasOwner, got:", err)
	}
	fmt.Println("OK")
	fmt.Printf("    Delete gs1 alias by itself: ")
	if err := node1gs1.DeleteAlias(alias); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	if a := node1gs1.Aliases(); len(a) > 0 {
		t.Fatal("alias table (registrar) must be empty on gs1 process", a)
	}

	if a := node1gs2.Aliases(); len(a) > 0 {
		t.Fatal("alias table (process) must be empty on gs2 process", a)

	}
	fmt.Printf("    Aliases must be cleaned up once the owner is down: ")
	alias1, _ := node1gs1.CreateAlias()
	alias2, _ := node1gs1.CreateAlias()
	alias3, _ := node1gs1.CreateAlias()
	if a := node1gs1.Aliases(); len(a) != 3 {
		t.Fatal("alias table of gs1 must have 3 aliases", a)
	}

	if !node1.IsAlias(alias1) || !node1.IsAlias(alias2) || !node1.IsAlias(alias3) {
		t.Fatal("not an alias", alias1, alias2, alias3)
	}

	node1gs1.Kill()
	time.Sleep(100 * time.Millisecond)
	if a := node1gs1.Aliases(); len(a) != 0 {
		t.Fatal("alias table  must be empty", a)
	}
	fmt.Println("OK")

	fmt.Printf("    Create gs1 alias on a stopped process (shouldn't be allowed): ")
	alias, err = node1gs1.CreateAlias()
	if err != node.ErrProcessTerminated {
		t.Fatal(err)
	}
	fmt.Println("OK")

}
