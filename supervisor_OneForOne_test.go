package ergonode

// - Supervisor

//  - one for one (permanent)
//    start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3
//    gs1.stop(normal) (sv1 restarting gs1)
//    gs2.stop(shutdown) (sv1 restarting gs2)
//    gs3.stop(panic) (sv1 restarting gs3)

//  - one for one (transient)
//    start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3
//    gs1.stop(normal) (sv1 wont restart gs1)
//    gs2.stop(shutdown) (sv1 wont restart gs2)
//    gs3.stop(panic) (sv1 restarting gs3 only)

//  - one for one (temporary)
//    start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3
//    gs1.stop(normal) (sv1 wont restart gs1)
//    gs2.stop(shutdown) (sv1 wont restart gs2)
//    gs3.stop(panic) (sv1 wont gs3 only)

import (
	"fmt"
	"testing"
	"time"

	"github.com/halturin/ergonode/etf"
)

type testSupervisorOneForOne struct {
	Supervisor
}

type testSupervisorGenServer struct {
	GenServer
	process Process
	v       chan interface{}
}

func (tsv *testSupervisorGenServer) Init(p Process, args ...interface{}) (state interface{}) {
	tsv.process = p
	fmt.Printf("\ntestSupervisorGenServer ({%s, %s}): Init\n", tsv.process.name, tsv.process.Node.FullName)
	// tsv.v <- p.Self()

	return nil
}
func (tsv *testSupervisorGenServer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("testSupervisorGenServer ({%s, %s}): HandleCast: %#v\n", tsv.process.name, tsv.process.Node.FullName, message)
	// tsv.v <- message
	return "stop", "test"
	// return "noreply", state
}
func (tsv *testSupervisorGenServer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	// fmt.Printf("testSupervisorGenServer ({%s, %s}): HandleCall: %#v, From: %#v\n", tsv.process.name, tsv.process.Node.FullName, message, from)
	return "reply", message, state
}
func (tsv *testSupervisorGenServer) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	// fmt.Printf("testSupervisorGenServer ({%s, %s}): HandleInfo: %#v\n", tsv.process.name, tsv.process.Node.FullName, message)
	// tsv.v <- message
	return "noreply", state
}
func (tsv *testSupervisorGenServer) Terminate(reason string, state interface{}) {
	fmt.Printf("\ntestSupervisorGenServer ({%s, %s}): Terminate: %#v\n", tsv.process.name, tsv.process.Node.FullName, reason)
}

func TestSupervisorOneForOne(t *testing.T) {
	fmt.Printf("\n== Test Supervisor - one for one\n")
	fmt.Printf("Starting node nodeSvOneForOne@localhost: ")
	node := CreateNode("nodeSvOneForOne@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("Starting supervisor 'testSupervisorPermanent' (%s)... ", SupervisorChildRestartPermanent)
	sv := &testSupervisorOneForOne{}
	processSV, _ := node.Spawn("testSupervisorPermanent", ProcessOptions{}, sv, SupervisorChildRestartPermanent)
	fmt.Println("OK")

	processSV.Cast(etf.Tuple{"testGS1", "nodeSvOneForOne@localhost"}, "ok")
	time.Sleep(100 * time.Millisecond)

	processSV.Stop()
	time.Sleep(100 * time.Millisecond)

	processSV, _ = node.Spawn("testSupervisorTransient", ProcessOptions{}, sv, SupervisorChildRestartTransient)
	fmt.Printf("Started supervisor (%s): %v\n", SupervisorChildRestartTransient, processSV.Self())

	processSV.Stop()
	time.Sleep(100 * time.Millisecond)

	processSV, _ = node.Spawn("testSupervisorTemporary", ProcessOptions{}, sv, SupervisorChildRestartTemporary)
	fmt.Printf("Started supervisor (%s): %v\n", SupervisorChildRestartTemporary, processSV.Self())

	processSV.Stop()
	time.Sleep(100 * time.Millisecond)

}

func (ts *testSupervisorOneForOne) Init(args ...interface{}) SupervisorSpec {
	restart := args[0].(string)
	return SupervisorSpec{
		children: []SupervisorChildSpec{
			SupervisorChildSpec{
				name:    "testGS1",
				child:   &testSupervisorGenServer{},
				restart: restart,
			},
			SupervisorChildSpec{
				name:    "testGS2",
				child:   &testSupervisorGenServer{},
				restart: restart,
			},
			SupervisorChildSpec{
				name:    "testGS3",
				child:   &testSupervisorGenServer{},
				restart: restart,
			},
		},
		strategy: SupervisorStrategy{
			Type:      SupervisorStrategyOneForOne,
			Intensity: 10,
			Period:    5,
		},
	}
}
