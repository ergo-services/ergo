//+build monitor

package ergonode

import (
	"testing"
	"time"
)


func TestMonitor(t *testing.T) {
	node := CreateNode("node@localhost","cookies", NodeOptions{})

	g1:=&GenServer{}
	g2:=&GenServer{}
	g3:=&GenServer{}

	m := createMonitor(node)
	cr := createRegistrar(node)

	process1 := cr.RegisterProcess(g1)
	process2 := cr.RegisterProcess(g2)
	process3 := cr.RegisterProcess(g3)

	m.MonitorProcess(process1.Self(), process2.Self())
	m.MonitorProcess(process1.Self(), process2.Self())

	m.MonitorProcess(process2.Self(), process1.Self())
	process1.Stop()
	process3.Stop()
	time.Sleep(1 *time.Second)
	node.Stop()
	time.Sleep(1 *time.Second)

}
