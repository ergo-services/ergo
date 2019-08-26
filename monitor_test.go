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

	process1 := node.registrar.RegisterProcess(g1)
	process2 := node.registrar.RegisterProcess(g2)
	process3 := node.registrar.RegisterProcess(g3)

	node.monitor.MonitorProcess(process1.Self(), process2.Self())
	node.monitor.MonitorProcess(process1.Self(), process2.Self())

	node.monitor.MonitorProcess(process3.Self(), process2.Self())
	node.monitor.MonitorProcess(process1.Self(), process3.Self())

	process2.Stop("normal")
	process3.Stop("normal")
	time.Sleep(1 *time.Second)
	node.Stop()
	time.Sleep(1 *time.Second)

}
