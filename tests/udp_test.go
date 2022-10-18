package tests

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

var (
	resUDPChan = make(chan interface{}, 2)
)

type testUDPHandler struct {
	gen.UDPHandler
}

func (r *testUDPHandler) HandlePacket(process *gen.UDPHandlerProcess, data []byte, packet gen.UDPPacket) {
	resUDPChan <- data
	return
}

type testUDPServer struct {
	gen.UDP
}

func (ts *testUDPServer) InitUDP(process *gen.UDPProcess, args ...etf.Term) (gen.UDPOptions, error) {
	var options gen.UDPOptions
	options.Handler = &testUDPHandler{}
	options.Port = 10101

	return options, nil
}

func TestUDP(t *testing.T) {
	fmt.Printf("\n=== Test UDP Server\n")
	fmt.Printf("Starting nodes: nodeUDP1@localhost: ")
	node1, err := ergo.StartNode("nodeUDP1@localhost", "cookies", node.Options{})
	defer node1.Stop()
	if err != nil {
		t.Fatal("can't start node", err)
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("...starting process (gen.UDP): ")
	udpProcess, err := node1.Spawn("udp", gen.ProcessOptions{}, &testUDPServer{})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("...send/receive data: ")
	c, err := net.Dial("udp", "localhost:10101")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	data := []byte{1, 2, 3, 4, 5}
	c.Write(data)
	waitForResultWithValue(t, resUDPChan, data)

	fmt.Printf("...stopping process (gen.UDP): ")
	udpProcess.Kill()
	if err := udpProcess.WaitWithTimeout(time.Second); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
}
