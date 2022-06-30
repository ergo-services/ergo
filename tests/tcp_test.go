package tests

import (
	"fmt"
	"testing"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

var (
	testString = "hello world"
)

type testTCPHandler struct {
	gen.TCPHandler
}

func (r *testTCPHandler) HandlePacket(process *gen.TCPHandlerProcess, packet []byte, conn gen.TCPConnection) (int, gen.TCPHandlerStatus) {
	return 0, gen.TCPHandlerStatusOK
}

type testTCPServer struct {
	gen.TCP
}

func (ts *testTCPServer) InitTCP(process *gen.TCPProcess, args ...etf.Term) (gen.TCPOptions, error) {
	var options gen.TCPOptions
	options.Handler = &testTCPHandler{}
	options.Port = 10101

	return options, nil
}

func TestTCP(t *testing.T) {
	fmt.Printf("\n=== Test TCP Server\n")
	fmt.Printf("Starting nodes: nodeTCP1@localhost: ")
	node1, err := ergo.StartNode("nodeTCP1@localhost", "cookies", node.Options{})
	defer node1.Stop()
	if err != nil {
		t.Fatal("can't start node", err)
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("...starting process (gen.TCP): ")
	_, err = node1.Spawn("web", gen.ProcessOptions{}, &testTCPServer{})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

}
