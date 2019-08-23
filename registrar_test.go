//+build registrar

package ergonode

import (
	"testing"
	"time"
)


func TestCreateRegistrar(t *testing.T) {
	node := CreateNode("node@localhost","cookies", NodeOptions{})

	g:=&GenServer{}
	cr := createRegistrar(node)
	p := cr.RegisterProcess(g)
	g.Process = p
	t.Logf("registered processes: %#v \n", cr.Registered())
	time.Sleep(1 *time.Second)
	t.Logf("processes: %#v \n", p.self)
	p.Stop()

}
