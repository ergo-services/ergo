//+build supervisor

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

// - one for rest (permanent)
//     start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3

//    gs2.stop(panic) (sv1 stopping gs2, gs3)
//               (sv1 starting gs2,gs3)
//    gs1.stop(panic) (sv1 stopping gs1, gs2, gs3)
//               (sv1 starting gs1, gs2,gs3)
//    gs3.stop(panic) (sv1 stopping gs3)
//               (sv1 starting gs3)

import (
	"testing"
	"time"
) 

type testSupervisor struct {
	Supervisor
}

type testGenServer1 struct {
	GenServer
}

type testGenServer2 struct {
	GenServer
}

func TestSupervisor1(t *testing.T) {
	 CreateNode("","", NodeOptions{})

	 node := CreateNode("node@localhost","cookies", NodeOptions{})
	 g:=&testSupervisor{}
	 process_opts := map[string]interface{}{
		 "mailbox-size": DefaultProcessMailboxSize, // size of channel for regular messages
	 }
	 pp := node.Spawn("testSupervisor", process_opts, g)
	 time.Sleep(1 *time.Second)
	 pp.Stop()
	 time.Sleep(1 *time.Second)
	 node.Stop()
	 time.Sleep(1 *time.Second)
}


func (ts *testSupervisor) Init() Supervisor{

}