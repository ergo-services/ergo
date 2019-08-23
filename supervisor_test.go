//+build supervisor

package ergonode

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