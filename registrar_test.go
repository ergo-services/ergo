//+build registrar

package ergonode

import (
	"testing"
	"context"
	"fmt"
	"time"
)


func TestCreateRegistrar(t *testing.T) {
	g:=&GenServer{}
	ctx, cancel := context.WithCancel(context.Background())
	cr := createRegistrar(ctx, "testRegistrar")
	process := cr.RegisterProcess(g)
	fmt.Printf("DDD %#v \n", cr.Registered())
	time.Sleep(1 *time.Second)
	// cancel1()
	// time.Sleep(1 *time.Second)
	process.stop()
	time.Sleep(1 *time.Second)
	cancel()
	time.Sleep(1 *time.Second)

}
