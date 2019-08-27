//+build monitor

package ergonode

import (
	"testing"
	"time"
	"github.com/halturin/ergonode/etf"
)

type TestServer struct {
	GenServer

}

func TestMonitor(t *testing.T) {
	node := CreateNode("node@localhost","cookies", NodeOptions{})

	g1:=&observer{}
	g2:=&observer{}
	g3:=&GenServer{}

	process1 := node.registrar.RegisterProcess(g1)
	process2 := node.registrar.RegisterProcess(g2)
	process3 := node.registrar.RegisterProcess(g3)

	node.monitor.MonitorProcess(process1.Self(), process2.Self())
	node.monitor.MonitorProcess(process1.Self(), process2.Self())

	process1.Send(process2.Self(), etf.Term(etf.Atom("hi")))
	process1.Send("observer", etf.Term(etf.Atom("hi observer")))
	process1.Send(etf.Tuple{"net_kernel", "node@localhost"}, etf.Term(etf.Atom("hi kernel")))

	node.monitor.MonitorProcess(process3.Self(), process2.Self())
	node.monitor.MonitorProcess(process1.Self(), process3.Self())

	process2.Stop("normal")
	process3.Stop("normal")
	time.Sleep(1 *time.Second)
	node.Stop()
	time.Sleep(1 *time.Second)

}
