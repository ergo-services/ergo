//+build supervisor

package ergonode

import (
	"testing"
	"time"
) 

func TestCreateSupervisor(t *testing.T) {
	 CreateNode("","", NodeOptions{})

	gs := []*GenServer{}
	sv := CreateSupervisor(gs, SupervisorStrategyOneForAll, 5, 10)
	if sv == nil {
		t.Error("can't create supervisor")
	}

	time.Sleep(time.Second *3)
	sv.Stop()
	time.Sleep(time.Second *1)

}
