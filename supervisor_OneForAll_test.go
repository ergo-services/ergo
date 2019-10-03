
package ergonode


// - Supervisor

// - one for all (permanent)
//    start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3
//    gs1.stop(normal) (sv1 stoping gs1)
//                     (sv1 stoping gs2,gs3)
//                     (sv1 starting gs1,gs2,gs3)
//    gs2.stop(shutdown) (sv1 stoping gs2)
//                     (sv1 stoping gs1,gs3)
//                     (sv1 starting gs1,gs2,gs3)
//    gs3.stop(panic) (sv1 stoping gs3)
//                     (sv1 stoping gs1,gs2)
//                     (sv1 starting gs1,gs2,gs3)

// - one for all (transient)
//    start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3
//    gs3.stop(panic) (sv1 stoping gs3)
//                     (sv1 stopping gs1, gs2)
//                     (sv1 starting gs1, gs2, gs3)

//    gs1.stop(normal) (sv1 stoping gs1)
//                     ( gs2, gs3 - still working)
//    gs2.stop(shutdown) (sv1 stoping gs2)
//                     (gs3 - still working)

// - one for all (temoporary)
//   start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3

//    gs3.stop(panic) (sv1 stoping gs3)
//                     (sv1 stopping gs1, gs2)

//    start again gs1, gs2, gs3 via sv1
//    gs1.stop(normal) (sv1 stopping gs1)
//                     (gs2, gs3 are still running)
//    gs2.stop(shutdown) (sv1 stopping gs2)
//                     (gs3 are still running)

import (
	"fmt"
	"testing"
	"time"

	"github.com/halturin/ergonode/etf"
)

type testSupervisorOneForAll struct {
	Supervisor
}


func TestSupervisorOneForAll(t *testing.T) {
	fmt.Printf("\n== Test Supervisor - one for all\n")
	fmt.Printf("Starting node nodeSvOneForAll@localhost: ")
	node := CreateNode("nodeSvOneForAll@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("Starting supervisor 'testSupervisorPermanent' (%s)... ", SupervisorChildRestartPermanent)
	sv := &testSupervisorOneForAll{}
	processSV, _ := node.Spawn("testSupervisorPermanent", ProcessOptions{}, sv, SupervisorChildRestartPermanent)
	fmt.Println("OK")

	processSV.Cast(etf.Tuple{"testGS2", "nodeSvOneForAll@localhost"}, "ok")
	time.Sleep(5 * time.Second)

	processSV.Exit(etf.Pid{}, "normal")
	time.Sleep(100 * time.Millisecond)

	processSV, _ = node.Spawn("testSupervisorTransient", ProcessOptions{}, sv, SupervisorChildRestartTransient)
	fmt.Printf("Started supervisor (%s): %v\n", SupervisorChildRestartTransient, processSV.Self())

	processSV.Exit(etf.Pid{}, "normal")
	time.Sleep(100 * time.Millisecond)

	processSV, _ = node.Spawn("testSupervisorTemporary", ProcessOptions{}, sv, SupervisorChildRestartTemporary)
	fmt.Printf("Started supervisor (%s): %v\n", SupervisorChildRestartTemporary, processSV.Self())

	processSV.Exit(etf.Pid{}, "normal")
	time.Sleep(100 * time.Millisecond)

}

func (ts *testSupervisorOneForAll) Init(args ...interface{}) SupervisorSpec {
	restart := args[0].(string)
	return SupervisorSpec{
		Children: []SupervisorChildSpec{
			SupervisorChildSpec{
				Name:    "testGS1",
				Child:   &testSupervisorGenServer{},
				Restart: restart,
			},
			SupervisorChildSpec{
				Name:    "testGS2",
				Child:   &testSupervisorGenServer{},
				Restart: restart,
			},
			SupervisorChildSpec{
				Name:    "testGS3",
				Child:   &testSupervisorGenServer{},
				Restart: restart,
			},
		},
		Strategy: SupervisorStrategy{
			Type:      SupervisorStrategyOneForAll,
			Intensity: 10,
			Period:    5,
		},
	}
}
