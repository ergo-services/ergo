//+build monitor

package ergonode

import (
	"testing"
	"time"
	"fmt"
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
	fmt.Println(111)
	process1.Send(process2.Self(), etf.Term(etf.Atom("hi")))
	process1.Send("observer", etf.Term(etf.Atom("hi observer")))
	process1.Send(etf.Tuple{"net_kernel", "node@localhost"}, etf.Term(etf.Atom("hi kernel")))
	fmt.Println(222)

	process1.Send(etf.Tuple{"abc", "node01@localhost"}, etf.Term(etf.Atom("hi remote kernel")))

	fmt.Println(333)

	node.monitor.MonitorProcess(process3.Self(), process2.Self())
	node.monitor.MonitorProcess(process1.Self(), process3.Self())
	fmt.Println(555)

	process2.Stop("normal")
	process3.Stop("normal")
	fmt.Println(666)

	time.Sleep(1 *time.Second)
	node.Stop()
	time.Sleep(1 *time.Second)
	fmt.Println(777)

}
