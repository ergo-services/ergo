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
	// fmt.Printf("testSupervisorGenServer ({%s, %s}): HandleCast: %#v\n", tsv.process.name, tsv.process.Node.FullName, message)
	// tsv.v <- message
	return "noreply", state
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
	// fmt.Printf("\ntestSupervisorGenServer ({%s, %s}): Terminate: %#v\n", tsv.process.name, tsv.process.Node.FullName, reason)
}

func TestSupervisorOneForOne(t *testing.T) {
	node := CreateNode("nodeSvOneForOne@localhost", "cookies", NodeOptions{})

	sv := &testSupervisorOneForOne{}
	processSV, _ := node.Spawn("testSupervisor", ProcessOptions{}, sv)
	fmt.Printf("Started supervisor: %v\n", processSV.Self())
}

func (ts *testSupervisorOneForOne) Init(args ...interface{}) SupervisorSpec {
	return SupervisorSpec{
		children: []SupervisorChildSpec{
			SupervisorChildSpec{
				name:    "testGS1",
				child:   &testSupervisorGenServer{},
				restart: SupervisorChildRestartPermanent,
			},
			SupervisorChildSpec{
				name:    "testGS2",
				child:   &testSupervisorGenServer{},
				restart: SupervisorChildRestartPermanent,
			},
			SupervisorChildSpec{
				name:    "testGS3",
				child:   &testSupervisorGenServer{},
				restart: SupervisorChildRestartPermanent,
			},
		},
		strategy: SupervisorStrategy{
			Type:      SupervisorStrategyOneForOne,
			Intensity: 10,
			Period:    5,
		},
	}
}
