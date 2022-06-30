package tests

import (
	"fmt"
	"net"
	"testing"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

var (
	resChan = make(chan interface{}, 2)
)

type testTCPHandler struct {
	gen.TCPHandler
}

type messageTestTCPConnect struct{}
type messageTestTCPStatusNext struct {
	left  int
	await int
}

func (r *testTCPHandler) HandleConnect(process *gen.TCPHandlerProcess, conn gen.TCPConnection) gen.TCPHandlerStatus {
	resChan <- messageTestTCPConnect{}
	return gen.TCPHandlerStatusOK
}

func (r *testTCPHandler) HandlePacket(process *gen.TCPHandlerProcess, packet []byte, conn gen.TCPConnection) (int, int, gen.TCPHandlerStatus) {
	l := len(packet)
	//fmt.Println("GOT", process.Self(), packet, l, "bytes", l%10)
	if l < 10 {
		resChan <- messageTestTCPStatusNext{
			left:  l,
			await: 10 - l,
		}
		return l, 10 - l, gen.TCPHandlerStatusNext
	}
	if l > 10 {
		resChan <- messageTestTCPStatusNext{
			left:  l % 10,
			await: 10 - (l % 10),
		}
		return l % 10, 10 - (l % 10), gen.TCPHandlerStatusNext
	}
	resChan <- packet
	return 0, 0, gen.TCPHandlerStatusOK
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
	tcpProcess, err := node1.Spawn("tcp", gen.ProcessOptions{}, &testTCPServer{})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("...makeing a new connection: ")
	conn, err := net.Dial("tcp", "localhost:10101")
	if err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, resChan, messageTestTCPConnect{})

	fmt.Printf("...send/recv data (10 bytes as 1 logic dataframe): ")
	testData1 := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if _, err := conn.Write(testData1); err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, resChan, testData1)

	fmt.Printf("...send/recv data (7 bytes as a part of logic dataframe): ")
	testData2 := []byte{11, 12, 13, 14, 15, 16, 17}
	if _, err := conn.Write(testData2); err != nil {
		t.Fatal(err)
	}

	value := messageTestTCPStatusNext{
		left:  7,
		await: 3,
	}

	waitForResultWithValue(t, resChan, value)

	fmt.Printf("...send/recv data (5 bytes, must be 1 logic dataframe + extra 2 bytes): ")
	testData2 = []byte{18, 19, 20, 21, 22}
	if _, err := conn.Write(testData2); err != nil {
		t.Fatal(err)
	}
	value = messageTestTCPStatusNext{
		left:  2,
		await: 8,
	}
	waitForResultWithValue(t, resChan, value)

	fmt.Printf("...send/recv data (8 bytes, must be 1 logic dataframe): ")
	testData2 = []byte{23, 24, 25, 26, 27, 28, 29, 30}
	if _, err := conn.Write(testData2); err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, resChan, []byte{21, 22, 23, 24, 25, 26, 27, 28, 29, 30})

	tcpProcess.Kill()
	tcpProcess.Wait()

	fmt.Printf("...stopping process (gen.TCP): ")
	if _, err := net.Dial("tcp", "localhost:10101"); err == nil {
		t.Fatal("error must be here")
	}
	fmt.Println("OK")
}
