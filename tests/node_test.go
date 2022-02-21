package tests

import (
	"context"
	"crypto/md5"
	"fmt"
	"math/rand"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
	"github.com/ergo-services/ergo/proto/dist"
)

type benchCase struct {
	name  string
	value etf.Term
}

func TestNode(t *testing.T) {
	ctx := context.Background()
	opts := node.Options{
		Listen:   25001,
		Resolver: dist.CreateResolverWithLocalEPMD("", 24999),
	}

	node1, _ := ergo.StartNodeWithContext(ctx, "node@localhost", "cookies", opts)

	if conn, err := net.Dial("tcp", ":25001"); err != nil {
		fmt.Println("Connect to the node' listening port FAILED")
		t.Fatal(err)
	} else {
		defer conn.Close()
	}

	if conn, err := net.Dial("tcp", ":24999"); err != nil {
		fmt.Println("Connect to the node' listening EPMD port FAILED")
		t.Fatal(err)
	} else {
		defer conn.Close()
	}

	gs1 := &testServer{
		res: make(chan interface{}, 2),
	}
	p, e := node1.Spawn("", gen.ProcessOptions{}, gs1)
	if e != nil {
		t.Fatal(e)
	}

	if !p.IsAlive() {
		t.Fatal("IsAlive: expect 'true', but got 'false'")
	}

	_, ee := node1.ProcessInfo(p.Self())
	if ee != nil {
		t.Fatal(ee)
	}

	node1.Stop()
}

type testFragmentationGS struct {
	gen.Server
}

func (f *testFragmentationGS) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	md5original := message.(etf.Tuple)[0].(string)
	blob := message.(etf.Tuple)[1].([]byte)

	result := etf.Atom("ok")
	md5 := fmt.Sprint(md5.Sum(blob))
	if !reflect.DeepEqual(md5original, md5) {
		result = etf.Atom("mismatch")
	}

	return result, gen.ServerStatusOK
}

type makeCall struct {
	to      interface{}
	message interface{}
}
type makeCast struct {
	to      interface{}
	message interface{}
}

func (f *testFragmentationGS) HandleDirect(process *gen.ServerProcess, message interface{}) (interface{}, error) {
	switch m := message.(type) {
	case makeCall:
		return process.Call(m.to, m.message)
	}
	return nil, gen.ErrUnsupportedRequest
}

func TestNodeFragmentation(t *testing.T) {
	var wg sync.WaitGroup

	blob := make([]byte, 1024*1024)
	rand.Read(blob)
	md5 := fmt.Sprint(md5.Sum(blob))
	message := etf.Tuple{md5, blob}

	node1, _ := ergo.StartNode("nodeT1Fragmentation@localhost", "secret", node.Options{})
	node2, _ := ergo.StartNode("nodeT2Fragmentation@localhost", "secret", node.Options{})

	tgs := &testFragmentationGS{}
	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, tgs)
	p2, e2 := node2.Spawn("", gen.ProcessOptions{}, tgs)

	if e1 != nil {
		t.Fatal(e1)
	}
	if e2 != nil {
		t.Fatal(e2)
	}

	// check single call
	call := makeCall{
		to:      p2.Self(),
		message: message,
	}
	check, e := p1.Direct(call)
	if e != nil {
		t.Fatal(e)
	}
	if check != etf.Atom("ok") {
		t.Fatal("md5sum mismatch")
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			p1, _ := node1.Spawn("", gen.ProcessOptions{}, tgs)
			p2, _ := node2.Spawn("", gen.ProcessOptions{}, tgs)
			defer wg.Done()
			for k := 0; k < 100; k++ {
				call := makeCall{
					to:      p2.Self(),
					message: message,
				}
				check, e := p1.Direct(call)
				if e != nil {
					panic("err on call")
				}
				if check != etf.Atom("ok") {
					panic("md5sum mismatch")
				}
			}

		}()
	}
	wg.Wait()
}

func TestNodeStaticRoute(t *testing.T) {
	nodeName1 := "nodeT1StaticRoute@localhost"
	nodeName2 := "nodeT2StaticRoute@localhost"
	nodeStaticPort := uint16(9876)

	node1, e1 := ergo.StartNode(nodeName1, "secret", node.Options{})
	if e1 != nil {
		t.Fatal(e1)
	}
	defer node1.Stop()

	node2, e2 := ergo.StartNode(nodeName2, "secret", node.Options{})
	if e2 != nil {
		t.Fatal(e2)
	}
	defer node2.Stop()

	nr, err := node1.Resolve(nodeName2)
	if err != nil {
		t.Fatal("Can't resolve port number for ", nodeName2, err)
	}

	// override route for nodeName2 with static port
	e := node1.AddStaticRoutePort(nodeName2, nodeStaticPort, node.RouteOptions{})
	if e != nil {
		t.Fatal(e)
	}
	// should be overrided by the new value of nodeStaticPort
	if r, err := node1.Resolve(nodeName2); err != nil || r.Port != nodeStaticPort {
		t.Fatal("Wrong port number after adding static route. Got", r.Port, "Expected", nodeStaticPort)
	}

	node1.RemoveStaticRoute(nodeName2)

	// should be resolved into the original port number
	if nr2, err := node1.Resolve(nodeName2); err != nil || nr.Port != nr2.Port {
		t.Fatal("Wrong port number after removing static route")
	}
}

type handshakeGenServer struct {
	gen.Server
}

func (h *handshakeGenServer) Init(process *gen.ServerProcess, args ...etf.Term) error {
	return nil
}

func (h *handshakeGenServer) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return "pass", gen.ServerStatusOK
}
func (h *handshakeGenServer) HandleDirect(process *gen.ServerProcess, message interface{}) (interface{}, error) {
	switch m := message.(type) {
	case makeCall:
		return process.Call(m.to, m.message)
	}
	return nil, gen.ErrUnsupportedRequest
}

func TestNodeDistHandshake(t *testing.T) {
	fmt.Printf("\n=== Test Node Handshake versions\n")

	// handshake version 5
	handshake5options := dist.HandshakeOptions{
		Version: dist.HandshakeVersion5,
	}

	// handshake version 6
	handshake6options := dist.HandshakeOptions{
		Version: dist.HandshakeVersion6,
	}

	hgs := &handshakeGenServer{}

	type Pair struct {
		name  string
		nodeA node.Node
		nodeB node.Node
	}
	node1Options5 := node.Options{
		Handshake: dist.CreateHandshake(handshake5options),
	}
	node1, e1 := ergo.StartNode("node1Handshake5@localhost", "secret", node1Options5)
	if e1 != nil {
		t.Fatal(e1)
	}
	node2Options5 := node.Options{
		Handshake: dist.CreateHandshake(handshake5options),
	}
	node2, e2 := ergo.StartNode("node2Handshake5@localhost", "secret", node2Options5)
	if e2 != nil {
		t.Fatal(e2)
	}
	node3Options5 := node.Options{
		Handshake: dist.CreateHandshake(handshake5options),
	}
	node3, e3 := ergo.StartNode("node3Handshake5@localhost", "secret", node3Options5)
	if e3 != nil {
		t.Fatal(e3)
	}
	node4Options6 := node.Options{
		Handshake: dist.CreateHandshake(handshake6options),
	}
	node4, e4 := ergo.StartNode("node4Handshake6@localhost", "secret", node4Options6)
	if e4 != nil {
		t.Fatal(e4)
	}
	// node5, _ := ergo.StartNode("node5Handshake6@localhost", "secret", nodeOptions6)
	// node6, _ := ergo.StartNode("node6Handshake5@localhost", "secret", nodeOptions5)
	node7Options6 := node.Options{
		Handshake: dist.CreateHandshake(handshake6options),
	}
	node7, e7 := ergo.StartNode("node7Handshake6@localhost", "secret", node7Options6)
	if e7 != nil {
		t.Fatal(e7)
	}
	node8Options6 := node.Options{
		Handshake: dist.CreateHandshake(handshake6options),
	}
	node8, e8 := ergo.StartNode("node8Handshake6@localhost", "secret", node8Options6)
	if e8 != nil {
		t.Fatal(e8)
	}
	node9Options5WithTLS := node.Options{
		Handshake: dist.CreateHandshake(handshake5options),
		TLS:       node.TLS{Enable: true},
	}
	node9, e9 := ergo.StartNode("node9Handshake5@localhost", "secret", node9Options5WithTLS)
	if e9 != nil {
		t.Fatal(e9)
	}
	node10Options5WithTLS := node.Options{
		Handshake: dist.CreateHandshake(handshake5options),
		TLS:       node.TLS{Enable: true},
	}
	node10, e10 := ergo.StartNode("node10Handshake5@localhost", "secret", node10Options5WithTLS)
	if e10 != nil {
		t.Fatal(e10)
	}
	node11Options5WithTLS := node.Options{
		Handshake: dist.CreateHandshake(handshake5options),
		TLS:       node.TLS{Enable: true},
	}
	node11, e11 := ergo.StartNode("node11Handshake5@localhost", "secret", node11Options5WithTLS)
	if e11 != nil {
		t.Fatal(e11)
	}
	node12Options6WithTLS := node.Options{
		Handshake: dist.CreateHandshake(handshake6options),
		TLS:       node.TLS{Enable: true},
	}
	node12, e12 := ergo.StartNode("node12Handshake6@localhost", "secret", node12Options6WithTLS)
	if e12 != nil {
		t.Fatal(e12)
	}
	// node13, _ := ergo.StartNode("node13Handshake6@localhost", "secret", nodeOptions6WithTLS)
	// node14, _ := ergo.StartNode("node14Handshake5@localhost", "secret", nodeOptions5WithTLS)
	node15Options6WithTLS := node.Options{
		Handshake: dist.CreateHandshake(handshake6options),
		TLS:       node.TLS{Enable: true},
	}
	node15, e15 := ergo.StartNode("node15Handshake6@localhost", "secret", node15Options6WithTLS)
	if e15 != nil {
		t.Fatal(e15)
	}
	node16Options6WithTLS := node.Options{
		Handshake: dist.CreateHandshake(handshake6options),
		TLS:       node.TLS{Enable: true},
	}
	node16, e16 := ergo.StartNode("node16Handshake6@localhost", "secret", node16Options6WithTLS)
	if e16 != nil {
		t.Fatal(e16)
	}

	nodes := []Pair{
		{"No TLS. version 5 -> version 5", node1, node2},
		{"No TLS. version 5 -> version 6", node3, node4},
		//Pair{ "No TLS. version 6 -> version 5", node5, node6 },
		{"No TLS. version 6 -> version 6", node7, node8},
		{"With TLS. version 5 -> version 5", node9, node10},
		{"With TLS. version 5 -> version 6", node11, node12},
		//Pair{ "With TLS. version 6 -> version 5", node13, node14 },
		{"With TLS. version 6 -> version 6", node15, node16},
	}

	defer func(nodes []Pair) {
		for i := range nodes {
			nodes[i].nodeA.Stop()
			nodes[i].nodeB.Stop()
		}
	}(nodes)

	var pA, pB gen.Process
	var e error
	var result etf.Term
	for i := range nodes {
		pair := nodes[i]
		fmt.Printf("    %s %s -> %s: ", pair.name, pair.nodeA.Name(), pair.nodeB.Name())
		pA, e = pair.nodeA.Spawn("", gen.ProcessOptions{}, hgs)
		if e != nil {
			t.Fatal(e)
		}
		pB, e = pair.nodeB.Spawn("", gen.ProcessOptions{}, hgs)
		if e != nil {
			t.Fatal(e)
		}

		call := makeCall{
			to:      pB.Self(),
			message: "test",
		}
		result, e = pA.Direct(call)
		if e != nil {
			t.Fatal(e)
		}
		if r, ok := result.(string); !ok || r != "pass" {
			t.Fatal("wrong result")
		}
		fmt.Println("OK")
	}
}

func TestNodeRemoteSpawn(t *testing.T) {
	fmt.Printf("\n=== Test Node Remote Spawn\n")
	node1opts := node.Options{}
	node1opts.Proxy.Flags = node.DefaultProxyFlags()
	node1opts.Proxy.Flags.EnableRemoteSpawn = false

	node1, _ := ergo.StartNode("node1remoteSpawn@localhost", "secret", node1opts)
	node2opts := node.Options{}
	node2opts.Proxy.Transit = true
	node2, _ := ergo.StartNode("node2remoteSpawn@localhost", "secret", node2opts)
	node3, _ := ergo.StartNode("node3remoteSpawn@localhost", "secret", node.Options{})
	route := node.ProxyRoute{
		Proxy: node2.Name(),
	}
	node1.AddProxyRoute(node3.Name(), route)
	defer node1.Stop()
	defer node2.Stop()
	defer node3.Stop()

	if err := node1.Connect(node3.Name()); err != nil {
		t.Fatal(err)
	}

	node2.ProvideRemoteSpawn("remote", &handshakeGenServer{})
	process, err := node1.Spawn("gs1", gen.ProcessOptions{}, &handshakeGenServer{})
	if err != nil {
		t.Fatal(err)
	}

	opts := gen.RemoteSpawnOptions{
		Name: "remote",
	}
	fmt.Printf("    process gs1@node1 request to spawn new process on node2 and register this process with name 'remote': ")
	gotPid, err := process.RemoteSpawn(node2.Name(), "remote", opts, 1, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	p := node2.ProcessByName("remote")
	if p == nil {
		t.Fatal("can't find process 'remote' on node2")
	}
	if gotPid != p.Self() {
		t.Fatal("process pid mismatch")
	}
	fmt.Println("OK")

	fmt.Printf("    process gs1@node1 request to spawn new process on node2 with the same name (must be failed): ")
	_, err = process.RemoteSpawn(node2.Name(), "remote", opts, 1, 2, 3)
	if err != node.ErrTaken {
		t.Fatal(err)
	}
	fmt.Println("OK")
	fmt.Printf("    process gs1@node1 request to spawn new process on node2 with unregistered behavior name (must be failed): ")
	_, err = process.RemoteSpawn(node2.Name(), "randomname", opts, 1, 2, 3)
	if err != node.ErrBehaviorUnknown {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("    process gs1@node1 request to spawn new process on node3 via proxy node2 and register this process with name 'remote': ")
	node3.ProvideRemoteSpawn("remote", &handshakeGenServer{})
	gotPid, err = process.RemoteSpawn(node3.Name(), "remote", opts, 1, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	p = node3.ProcessByName("remote")
	if p == nil {
		t.Fatal("can't find process 'remote' on node2")
	}
	if gotPid != p.Self() {
		t.Fatal("process pid mismatch")
	}
	fmt.Println("OK")
	fmt.Printf("    process gs3@node3 request to spawn new process on node1 via proxy node2 (node1 ProxyFlags.RemoteSpawn: false): ")
	process3, err := node3.Spawn("gs3", gen.ProcessOptions{}, &handshakeGenServer{})
	if err != nil {
		t.Fatal(err)
	}
	gotPid, err = process3.RemoteSpawn(node1.Name(), "remote", opts, 1, 2, 3)
	if err != node.ErrPeerUnsupported {
		t.Fatal(err)
	}
	fmt.Println("OK")
}

func TestNodeResolveExtra(t *testing.T) {
	fmt.Printf("\n=== Test Node Resolve Extra \n")
	fmt.Printf("... starting node1 with disabled TLS: ")
	node1, err := ergo.StartNode("node1resolveExtra@localhost", "secret", node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()
	fmt.Println("OK")
	opts := node.Options{}
	opts.TLS.Enable = true
	fmt.Printf("... starting node2 with enabled TLS: ")
	node2, err := ergo.StartNode("node2resolveExtra@localhost", "secret", opts)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()
	fmt.Println("OK")

	fmt.Printf("... node1 resolves node2 with enabled TLS: ")
	route1, err := node1.Resolve("node2resolveExtra@localhost")
	if err != nil {
		t.Fatal(err)
	}
	if route1.Options.EnableTLS == false {
		t.Fatal("expected true value")
	}
	fmt.Println("OK")

	fmt.Printf("... node2 resolves node1 with disabled TLS: ")
	route2, err := node2.Resolve("node1resolveExtra@localhost")
	if err != nil {
		t.Fatal(err)
	}
	if route2.Options.EnableTLS == true {
		t.Fatal("expected true value")
	}
	fmt.Println("OK")

	fmt.Printf("... node1 connect to node2: ")
	if err := node1.Connect(node2.Name()); err != nil {
		t.Fatal(err)
	}
	if len(node1.Nodes()) != 1 {
		t.Fatal("no peers")
	}
	if node1.Nodes()[0] != node2.Name() {
		t.Fatal("wrong peer")
	}
	fmt.Println("OK")

	fmt.Printf("... disconnecting nodes: ")
	time.Sleep(300 * time.Millisecond)
	if err := node1.Disconnect(node2.Name()); err != nil {
		t.Fatal(err)
	}
	if len(node1.Nodes()) > 0 {
		t.Fatal("still connected")
	}
	fmt.Println("OK")

	fmt.Printf("... node2 connect to node1: ")
	if err := node2.Connect(node1.Name()); err != nil {
		t.Fatal(err)
	}
	if len(node2.Nodes()) != 1 {
		t.Fatal("no peers")
	}
	if node2.Nodes()[0] != node1.Name() {
		t.Fatal("wrong peer")
	}
	fmt.Println("OK")
}

type failoverServer struct {
	gen.Server
	v chan interface{}
}

func (f *failoverServer) Init(process *gen.ServerProcess, args ...etf.Term) error {
	return nil
}
func (f *failoverServer) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	if _, yes := gen.IsMessageFallback(message); yes {
		f.v <- message
		return gen.ServerStatusOK
	}
	time.Sleep(300 * time.Millisecond)
	return gen.ServerStatusOK
}
func TestNodeProcessFallback(t *testing.T) {
	fmt.Printf("\n=== Test Node Process Fallback\n")
	fmt.Printf("... start node1: ")
	node1, e := ergo.StartNode("node1processfallback@localhost", "secret", node.Options{})
	if e != nil {
		t.Fatal(e)
	}
	defer node1.Stop()
	fmt.Println("OK")
	popts1 := gen.ProcessOptions{
		MailboxSize: 2,
		Fallback: gen.ProcessFallback{
			Name: "fp",
			Tag:  "test_tag",
		},
	}
	gsf := &failoverServer{
		v: make(chan interface{}, 2),
	}

	fmt.Printf("... start process p1 (with mailbox size = 2 and fallback process = \"fp\"): ")
	p1, err := node1.Spawn("", popts1, &failoverServer{})
	if err != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")
	fmt.Printf("... start failover process p2 (with name = \"fp\"): ")
	_, err = node1.Spawn("fp", gen.ProcessOptions{}, gsf)
	if err != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")
	fmt.Printf("... sending 4 messages to p1 (4th must wrapped into gen.MessageFallback and forwarded to \"fp\" ): ")
	p1.Send(p1.Self(), "m1")
	p1.Send(p1.Self(), "m2")
	p1.Send(p1.Self(), "m3")
	// bellow message must be forwarded
	p1.Send(p1.Self(), "m4")

	result := gen.MessageFallback{Process: p1.Self(), Tag: "test_tag", Message: "m4"}
	waitForResultWithValue(t, gsf.v, result)
}

type compressionServer struct {
	gen.Server
}

func (c *compressionServer) Init(process *gen.ServerProcess, args ...etf.Term) error {
	return nil
}

func (c *compressionServer) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	blob := message.(etf.Tuple)[1].([]byte)
	md5original := message.(etf.Tuple)[0].(string)
	md5sum := fmt.Sprint(md5.Sum(blob))
	result := etf.Atom("ok")
	if !reflect.DeepEqual(md5original, md5sum) {
		result = etf.Atom("mismatch")
	}
	return result, gen.ServerStatusOK
}
func (c *compressionServer) HandleDirect(process *gen.ServerProcess, message interface{}) (interface{}, error) {
	switch m := message.(type) {
	case makeCall:
		return process.Call(m.to, m.message)
	}
	return nil, gen.ErrUnsupportedRequest
}
func TestNodeCompression(t *testing.T) {
	fmt.Printf("\n=== Test Node Compression \n")
	opts1 := node.Options{}
	opts1.Compression.Enable = true
	// need 1 handler to make Atom cache work
	protoOptions := node.DefaultProtoOptions()
	protoOptions.NumHandlers = 1
	opts1.Proto = dist.CreateProto(protoOptions)
	node1, e := ergo.StartNode("node1compression@localhost", "secret", opts1)
	if e != nil {
		t.Fatal(e)
	}
	defer node1.Stop()
	node2, e := ergo.StartNode("node2compression@localhost", "secret", node.Options{})
	if e != nil {
		t.Fatal(e)
	}
	defer node2.Stop()

	n1p1, err := node1.Spawn("", gen.ProcessOptions{}, &compressionServer{})
	if err != nil {
		t.Fatal(err)
	}
	n2p1, err := node2.Spawn("", gen.ProcessOptions{}, &compressionServer{})
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("... send 1MB compressed. no fragmentation: ")
	// empty data (no fragmentation)
	blob := make([]byte, 1024*1024)
	md5sum := fmt.Sprint(md5.Sum(blob))
	message := etf.Tuple{md5sum, blob}

	// send 3 times. that is how atom cache is working -
	// atoms are encoding from cache on 2nd or 3rd sending
	call := makeCall{
		to:      n2p1.Self(),
		message: message,
	}
	for i := 0; i < 3; i++ {
		result, e := n1p1.Direct(call)
		if e != nil {
			t.Fatal(e)
		}
		if result != etf.Atom("ok") {
			t.Fatal(result)
		}
	}
	fmt.Println("OK")

	fmt.Printf("... send 1MB compressed. with fragmentation: ")
	// will be fragmented
	rnd := lib.RandomString(1024 * 1024)
	blob = []byte(rnd) // compression rate for random string around 50%
	//rand.Read(blob[:66000]) // compression rate for 1MB of random data - 0 % (entropy too big)
	md5sum = fmt.Sprint(md5.Sum(blob))
	message = etf.Tuple{md5sum, blob}

	call = makeCall{
		to:      n2p1.Self(),
		message: message,
	}
	for i := 0; i < 3; i++ {
		result, e := n1p1.Direct(call)
		if e != nil {
			t.Fatal(e)
		}
		if result != etf.Atom("ok") {
			t.Fatal(result)
		}
	}
	fmt.Println("OK")
}

func TestNodeProxyConnect(t *testing.T) {
	fmt.Printf("\n=== Test Node Proxy\n")
	fmt.Printf("... connect NodeA to NodeC via NodeB: ")
	optsA := node.Options{}
	nodeA, e := ergo.StartNode("nodeAproxy@localhost", "secret", optsA)
	if e != nil {
		t.Fatal(e)
	}
	route := node.ProxyRoute{
		Proxy: "nodeBproxy@localhost",
	}
	nodeA.AddProxyRoute("nodeCproxy@localhost", route)

	optsB := node.Options{}
	optsB.Proxy.Transit = true
	nodeB, e := ergo.StartNode("nodeBproxy@localhost", "secret", optsB)
	if e != nil {
		t.Fatal(e)
	}
	optsC := node.Options{}
	nodeC, e := ergo.StartNode("nodeCproxy@localhost", "secret", optsC)
	if e != nil {
		t.Fatal(e)
	}

	if err := nodeA.Connect("nodeCproxy@localhost"); err != nil {
		t.Fatal(err)
	}

	indirectNodes := nodeA.NodesIndirect()
	if len(indirectNodes) != 1 {
		t.Fatal("wrong result:", indirectNodes)
	}
	if indirectNodes[0] != "nodeCproxy@localhost" {
		t.Fatal("wrong result:", indirectNodes)
	}
	indirectNodes = nodeC.NodesIndirect()
	if len(indirectNodes) != 1 {
		t.Fatal("wrong result:", indirectNodes)
	}
	if indirectNodes[0] != "nodeAproxy@localhost" {
		t.Fatal("wrong result:", indirectNodes)
	}
	if len(nodeB.NodesIndirect()) > 0 {
		t.Fatal("wrong result:", nodeB.NodesIndirect())
	}
	fmt.Println("OK")

	fmt.Printf("... disconnect NodeC from NodeA: ")
	nodeC.Disconnect("nodeAproxy@localhost")
	if len(nodeC.NodesIndirect()) > 0 {
		t.Fatal("wrong result:", nodeC.NodesIndirect())
	}

	time.Sleep(100 * time.Millisecond)
	if len(nodeA.NodesIndirect()) > 0 {
		t.Fatal("wrong result:", nodeA.NodesIndirect())
	}
	fmt.Println("OK")
	nodeB.Stop()
	optsB.Proxy.Transit = false
	nodeB, e = ergo.StartNode("nodeBproxy@localhost", "secret", optsB)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Printf("... connect NodeA to NodeC via NodeB(transit proxy disabled): ")
	e = nodeA.Connect("nodeCproxy@localhost")
	if e == nil {
		t.Fatal("must be error here")
	}
	errMessage := "[nodeBproxy@localhost] proxy feature disabled"
	if e.Error() != errMessage {
		t.Fatal(e)
	}
	fmt.Println("OK")
	nodeB.Stop()
	nodeC.Stop()

	nodeB.Stop()
	optsB.Proxy.Transit = true
	nodeB, e = ergo.StartNode("nodeBproxy@localhost", "secret", optsB)
	if e != nil {
		t.Fatal(e)
	}

	optsC.Flags = node.DefaultFlags()
	optsC.Flags.EnableProxy = false
	nodeC, e = ergo.StartNode("nodeCproxy@localhost", "secret", optsC)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Printf("... connect NodeA to NodeC (proxy feature support disabled) via NodeB: ")
	e = nodeA.Connect("nodeCproxy@localhost")
	if e == nil {
		t.Fatal("must be error here")
	}
	errMessage = "[nodeBproxy@localhost] peer does not support this feature"
	if e.Error() != errMessage {
		t.Fatal(e)
	}
	fmt.Println("OK")
	nodeC.Stop()

	optsC = node.Options{}
	optsC.Proxy.Cookie = "123"
	nodeC, e = ergo.StartNode("nodeCproxy@localhost", "secret", optsC)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Printf("... connect NodeA to NodeC (with wrong cookie) via NodeB: ")
	e = nodeA.Connect("nodeCproxy@localhost")
	if e == nil {
		t.Fatal("must be error here")
	}
	errMessage = "[nodeCproxy@localhost] can't establish proxy connection"
	if e.Error() != errMessage {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("... connect NodeA to NodeC (with correct cookie) via NodeB: ")
	if nodeA.RemoveProxyRoute("nodeCproxy@localhost") == false {
		t.Fatal("proxy route not found")
	}
	route = node.ProxyRoute{
		Proxy:  "nodeBproxy@localhost",
		Cookie: "123",
	}
	nodeA.AddProxyRoute("nodeCproxy@localhost", route)

	e = nodeA.Connect("nodeCproxy@localhost")
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("... connect NodeA to NodeD (with enabled encryption) via NodeB: ")
	optsD := node.Options{}
	optsD.Proxy.Cookie = "123"
	optsD.Proxy.Flags = node.DefaultProxyFlags()
	optsD.Proxy.Flags.EnableEncryption = true
	nodeD, e := ergo.StartNode("nodeDproxy@localhost", "secret", optsD)
	if e != nil {
		t.Fatal(e)
	}

	route = node.ProxyRoute{
		Proxy:  "nodeBproxy@localhost",
		Cookie: "123",
	}
	nodeA.AddProxyRoute("nodeDproxy@localhost", route)
	e = nodeA.Connect("nodeDproxy@localhost")
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	// use gen serv from test_monitor
	gsA := &testMonitor{
		v: make(chan interface{}, 2),
	}
	gsC := &testMonitor{
		v: make(chan interface{}, 2),
	}
	gsD := &testMonitor{
		v: make(chan interface{}, 2),
	}
	fmt.Printf("... start processA on NodeA: ")
	pA, err := nodeA.Spawn("", gen.ProcessOptions{}, gsA)
	if err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, gsA.v, pA.Self())
	fmt.Printf("... start processC on NodeC: ")
	pC, err := nodeC.Spawn("", gen.ProcessOptions{}, gsC)
	if err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, gsC.v, pC.Self())
	fmt.Printf("... start processD on NodeD: ")
	pD, err := nodeD.Spawn("", gen.ProcessOptions{}, gsD)
	if err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, gsD.v, pD.Self())

	fmt.Printf("... processA send short message to processC: ")
	pA.Send(pC.Self(), "test")
	waitForResultWithValue(t, gsC.v, "test")

	fmt.Printf("... processA send short message to processD (encrypted): ")
	pA.Send(pD.Self(), "test")
	waitForResultWithValue(t, gsD.v, "test")

	randomString := []byte(lib.RandomString(1024 * 10))
	pA.SetCompression(true)
	fmt.Printf("... processA send 10K message to processC (compressed): ")
	pA.Send(pC.Self(), randomString)
	waitForResultWithValue(t, gsC.v, randomString)

	fmt.Printf("... processA send 10K message to processD (compressed, encrypted): ")
	pA.Send(pD.Self(), randomString)
	waitForResultWithValue(t, gsD.v, randomString)

	pA.SetCompression(false)
	randomString = []byte(lib.RandomString(1024 * 100))
	fmt.Printf("... processA send 100K message to processC (fragmented): ")
	pA.Send(pC.Self(), randomString)
	waitForResultWithValue(t, gsC.v, randomString)

	fmt.Printf("... processA send 100K message to processD (fragmented, encrypted): ")
	pA.Send(pD.Self(), randomString)
	waitForResultWithValue(t, gsD.v, randomString)

	pA.SetCompression(true)
	randomString = []byte(lib.RandomString(1024 * 1024))
	fmt.Printf("... processA send 1M message to processC (fragmented, compressed): ")
	pA.Send(pC.Self(), randomString)
	waitForResultWithValue(t, gsC.v, randomString)

	fmt.Printf("... processA send 1M message to processD (fragmented, compressed, encrypted): ")
	pA.Send(pD.Self(), randomString)
	waitForResultWithValue(t, gsD.v, randomString)

	nodeA.Stop()
	nodeB.Stop()
	nodeC.Stop()
	nodeD.Stop()

}

func BenchmarkNodeCompressionDisabled1MBempty(b *testing.B) {
	node1name := fmt.Sprintf("nodeB1compressionDis_%d@localhost", b.N)
	node2name := fmt.Sprintf("nodeB2compressionDis_%d@localhost", b.N)
	node1, _ := ergo.StartNode(node1name, "bench", node.Options{})
	node2, _ := ergo.StartNode(node2name, "bench", node.Options{})
	defer node1.Stop()
	defer node2.Stop()
	if err := node1.Connect(node2.Name()); err != nil {
		b.Fatal(err)
	}

	bgs := &benchGS{}

	var empty [1024 * 1024]byte
	b.SetParallelism(15)
	b.RunParallel(func(pb *testing.PB) {
		p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
		if e1 != nil {
			b.Fatal(e1)
		}
		p2, e2 := node2.Spawn("", gen.ProcessOptions{}, bgs)
		if e2 != nil {
			b.Fatal(e2)
		}
		b.ResetTimer()
		for pb.Next() {
			call := makeCall{
				to:      p2.Self(),
				message: empty,
			}
			_, e := p1.DirectWithTimeout(call, 30)
			if e != nil {
				b.Fatal(e)
			}
		}

	})
}
func BenchmarkNodeCompressionEnabled1MBempty(b *testing.B) {
	node1name := fmt.Sprintf("nodeB1compressionEn_%d@localhost", b.N)
	node2name := fmt.Sprintf("nodeB2compressionEn_%d@localhost", b.N)
	node1, _ := ergo.StartNode(node1name, "bench", node.Options{})
	node2, _ := ergo.StartNode(node2name, "bench", node.Options{})
	defer node1.Stop()
	defer node2.Stop()
	if err := node1.Connect(node2.Name()); err != nil {
		b.Fatal(err)
	}

	bgs := &benchGS{}

	var empty [1024 * 1024]byte
	//b.SetParallelism(15)
	b.RunParallel(func(pb *testing.PB) {
		p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
		if e1 != nil {
			b.Fatal(e1)
		}
		p1.SetCompression(true)
		p1.SetCompressionLevel(5)
		p2, e2 := node2.Spawn("", gen.ProcessOptions{}, bgs)
		if e2 != nil {
			b.Fatal(e2)
		}
		b.ResetTimer()
		for pb.Next() {
			call := makeCall{
				to:      p2.Self(),
				message: empty,
			}
			_, e := p1.DirectWithTimeout(call, 30)
			if e != nil {
				b.Fatal(e)
			}
		}

	})
}

func BenchmarkNodeCompressionEnabled1MBstring(b *testing.B) {
	node1name := fmt.Sprintf("nodeB1compressionEnStr_%d@localhost", b.N)
	node2name := fmt.Sprintf("nodeB2compressionEnStr_%d@localhost", b.N)
	node1, e := ergo.StartNode(node1name, "bench", node.Options{})
	if e != nil {
		b.Fatal(e)
	}
	node2, e := ergo.StartNode(node2name, "bench", node.Options{})
	if e != nil {
		b.Fatal(e)
	}
	defer node1.Stop()
	defer node2.Stop()
	if err := node1.Connect(node2.Name()); err != nil {
		b.Fatal(err)
	}

	bgs := &benchGS{}

	randomString := []byte(lib.RandomString(1024 * 1024))
	b.SetParallelism(15)
	b.RunParallel(func(pb *testing.PB) {
		p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
		if e1 != nil {
			b.Fatal(e1)
		}
		p1.SetCompression(true)
		p1.SetCompressionLevel(5)
		p2, e2 := node2.Spawn("", gen.ProcessOptions{}, bgs)
		if e2 != nil {
			b.Fatal(e2)
		}
		b.ResetTimer()
		for pb.Next() {
			call := makeCall{
				to:      p2.Self(),
				message: randomString,
			}
			_, e := p1.DirectWithTimeout(call, 30)
			if e != nil {
				b.Fatal(e)
			}
		}

	})
}

type benchGS struct {
	gen.Server
}

func (b *benchGS) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return etf.Atom("ok"), gen.ServerStatusOK
}
func (b *benchGS) HandleDirect(process *gen.ServerProcess, message interface{}) (interface{}, error) {
	switch m := message.(type) {
	case makeCall:
		return process.CallWithTimeout(m.to, m.message, 30)
	}
	return nil, gen.ErrUnsupportedRequest
}

func BenchmarkNodeSequentialNetwork(b *testing.B) {

	node1name := fmt.Sprintf("nodeB1_%d@localhost", b.N)
	node2name := fmt.Sprintf("nodeB2_%d@localhost", b.N)
	node1, _ := ergo.StartNode(node1name, "bench", node.Options{})
	node2, _ := ergo.StartNode(node2name, "bench", node.Options{})

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
	p2, e2 := node2.Spawn("", gen.ProcessOptions{}, bgs)

	if e1 != nil {
		b.Fatal(e1)
	}
	if e2 != nil {
		b.Fatal(e2)
	}

	call := makeCall{
		to:      p2.Self(),
		message: 1,
	}
	if _, e := p1.Direct(call); e != nil {
		b.Fatal("single ping", e)
	}

	b.ResetTimer()
	for _, c := range benchCases() {
		b.Run(c.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				call := makeCall{
					to:      p2.Self(),
					message: c.value,
				}
				_, e := p1.Direct(call)
				if e != nil {
					b.Fatal(e, i)
				}
			}
		})
	}
}

func BenchmarkNodeSequentialLocal(b *testing.B) {

	node1name := fmt.Sprintf("nodeB1Local_%d@localhost", b.N)
	node1, _ := ergo.StartNode(node1name, "bench", node.Options{})

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
	p2, e2 := node1.Spawn("", gen.ProcessOptions{}, bgs)

	if e1 != nil {
		b.Fatal(e1)
	}
	if e2 != nil {
		b.Fatal(e2)
	}

	call := makeCall{
		to:      p2.Self(),
		message: 1,
	}
	if _, e := p1.Direct(call); e != nil {
		b.Fatal("single ping", e)
	}

	b.ResetTimer()
	for _, c := range benchCases() {
		b.Run(c.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				call := makeCall{
					to:      p2.Self(),
					message: c.value,
				}
				_, e := p1.Direct(call)
				if e != nil {
					b.Fatal(e, i)
				}
			}
		})
	}
}

func BenchmarkNodeParallel(b *testing.B) {

	node1name := fmt.Sprintf("nodeB1Parallel_%d@localhost", b.N)
	node2name := fmt.Sprintf("nodeB2Parallel_%d@localhost", b.N)
	node1, _ := ergo.StartNode(node1name, "bench", node.Options{})
	node2, _ := ergo.StartNode(node2name, "bench", node.Options{})

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
	if e1 != nil {
		b.Fatal(e1)
	}
	p2, e2 := node2.Spawn("", gen.ProcessOptions{}, bgs)
	if e2 != nil {
		b.Fatal(e2)
	}

	call := makeCall{
		to:      p2.Self(),
		message: "hi",
	}
	if _, e := p1.Direct(call); e != nil {
		b.Fatal("single ping", e)
	}

	b.SetParallelism(15)
	b.RunParallel(func(pb *testing.PB) {
		p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
		if e1 != nil {
			b.Fatal(e1)
		}
		p2, e2 := node2.Spawn("", gen.ProcessOptions{}, bgs)
		if e2 != nil {
			b.Fatal(e2)
		}
		b.ResetTimer()
		for pb.Next() {
			call := makeCall{
				to:      p2.Self(),
				message: etf.Atom("ping"),
			}
			_, e := p1.Direct(call)
			if e != nil {
				b.Fatal(e)
			}
		}

	})
}

func BenchmarkNodeParallelSingleNode(b *testing.B) {

	node1name := fmt.Sprintf("nodeB1ParallelLocal_%d@localhost", b.N)
	node1, _ := ergo.StartNode(node1name, "bench", node.Options{})

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
	if e1 != nil {
		b.Fatal(e1)
	}
	p2, e2 := node1.Spawn("", gen.ProcessOptions{}, bgs)
	if e2 != nil {
		b.Fatal(e2)
	}

	call := makeCall{
		to:      p2.Self(),
		message: "hi",
	}
	if _, e := p1.Direct(call); e != nil {
		b.Fatal("single ping", e)
	}
	b.SetParallelism(15)
	b.RunParallel(func(pb *testing.PB) {
		p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
		if e1 != nil {
			b.Fatal(e1)
		}
		p2, e2 := node1.Spawn("", gen.ProcessOptions{}, bgs)
		if e2 != nil {
			b.Fatal(e2)
		}
		b.ResetTimer()
		for pb.Next() {
			call := makeCall{
				to:      p2.Self(),
				message: etf.Atom("ping"),
			}
			_, e := p1.Direct(call)
			if e != nil {
				b.Fatal(e)
			}
		}

	})
}

func BenchmarkNodeProxy_NodeA_to_NodeC_direct_Message_1K(b *testing.B) {
	node1name := fmt.Sprintf("nodeB1ProxyDisabled%d@localhost", b.N)
	node2name := fmt.Sprintf("nodeB2ProxyDisabled%d@localhost", b.N)
	node1, _ := ergo.StartNode(node1name, "bench", node.Options{})
	node2, _ := ergo.StartNode(node2name, "bench", node.Options{})
	defer node1.Stop()
	defer node2.Stop()

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
	if e1 != nil {
		b.Fatal(e1)
	}
	p2, e2 := node2.Spawn("", gen.ProcessOptions{}, bgs)
	if e2 != nil {
		b.Fatal(e2)
	}

	call := makeCall{
		to:      p2.Self(),
		message: "hi",
	}
	if _, e := p1.Direct(call); e != nil {
		b.Fatal("single ping", e)
	}

	randomString := []byte(lib.RandomString(1024))
	b.SetParallelism(15)
	b.RunParallel(func(pb *testing.PB) {
		p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
		if e1 != nil {
			b.Fatal(e1)
		}
		p2, e2 := node2.Spawn("", gen.ProcessOptions{}, bgs)
		if e2 != nil {
			b.Fatal(e2)
		}
		b.ResetTimer()
		for pb.Next() {
			call := makeCall{
				to:      p2.Self(),
				message: randomString,
			}
			_, e := p1.Direct(call)
			if e != nil {
				b.Fatal(e)
			}
		}

	})
}
func BenchmarkNodeProxy_NodeA_to_NodeC_via_NodeB_Message_1K(b *testing.B) {
	node1name := fmt.Sprintf("nodeB1ProxyEnabled1K%d@localhost", b.N)
	node2name := fmt.Sprintf("nodeB2ProxyEnabled1K%d@localhost", b.N)
	node3name := fmt.Sprintf("nodeB3ProxyEnabled1K%d@localhost", b.N)
	node1, _ := ergo.StartNode(node1name, "bench", node.Options{})
	opts2 := node.Options{}
	opts2.Proxy.Transit = true
	node2, _ := ergo.StartNode(node2name, "bench", opts2)
	node3, _ := ergo.StartNode(node3name, "bench", node.Options{})
	defer node1.Stop()
	defer node2.Stop()
	defer node3.Stop()
	route := node.ProxyRoute{
		Proxy: node2.Name(),
	}
	node1.AddProxyRoute(node3.Name(), route)

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
	if e1 != nil {
		b.Fatal(e1)
	}
	p3, e3 := node3.Spawn("", gen.ProcessOptions{}, bgs)
	if e3 != nil {
		b.Fatal(e3)
	}

	call := makeCall{
		to:      p3.Self(),
		message: "hi",
	}
	if _, e := p1.Direct(call); e != nil {
		b.Fatal("single ping", e)
	}

	randomString := []byte(lib.RandomString(1024))
	b.SetParallelism(15)
	b.RunParallel(func(pb *testing.PB) {
		p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
		if e1 != nil {
			b.Fatal(e1)
		}
		p3, e3 := node3.Spawn("", gen.ProcessOptions{}, bgs)
		if e3 != nil {
			b.Fatal(e3)
		}
		b.ResetTimer()
		for pb.Next() {
			call := makeCall{
				to:      p3.Self(),
				message: randomString,
			}
			_, e := p1.Direct(call)
			if e != nil {
				b.Fatal(e)
			}
		}

	})
}

func BenchmarkNodeProxy_NodeA_to_NodeC_via_NodeB_Message_1K_Encrypted(b *testing.B) {
	node1name := fmt.Sprintf("nodeB1ProxyEnabled1K%d@localhost", b.N)
	node2name := fmt.Sprintf("nodeB2ProxyEnabled1K%d@localhost", b.N)
	node3name := fmt.Sprintf("nodeB3ProxyEnabled1K%d@localhost", b.N)
	opts1 := node.Options{}
	opts1.Proxy.Flags = node.DefaultProxyFlags()
	opts1.Proxy.Flags.EnableEncryption = true
	node1, _ := ergo.StartNode(node1name, "bench", opts1)
	opts2 := node.Options{}
	opts2.Proxy.Transit = true
	node2, _ := ergo.StartNode(node2name, "bench", opts2)
	node3, _ := ergo.StartNode(node3name, "bench", node.Options{})
	defer node1.Stop()
	defer node2.Stop()
	defer node3.Stop()
	route := node.ProxyRoute{
		Proxy: node2.Name(),
	}
	node1.AddProxyRoute(node3.Name(), route)

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
	if e1 != nil {
		b.Fatal(e1)
	}
	p3, e3 := node3.Spawn("", gen.ProcessOptions{}, bgs)
	if e3 != nil {
		b.Fatal(e3)
	}

	call := makeCall{
		to:      p3.Self(),
		message: "hi",
	}
	if _, e := p1.Direct(call); e != nil {
		b.Fatal("single ping", e)
	}

	randomString := []byte(lib.RandomString(1024))
	b.SetParallelism(15)
	b.RunParallel(func(pb *testing.PB) {
		p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
		if e1 != nil {
			b.Fatal(e1)
		}
		p3, e3 := node3.Spawn("", gen.ProcessOptions{}, bgs)
		if e3 != nil {
			b.Fatal(e3)
		}
		b.ResetTimer()
		for pb.Next() {
			call := makeCall{
				to:      p3.Self(),
				message: randomString,
			}
			_, e := p1.Direct(call)
			if e != nil {
				b.Fatal(e)
			}
		}

	})
}

func BenchmarkNodeProxy_NodeA_to_NodeC_via_NodeB_Message_1M_Compressed(b *testing.B) {
	node1name := fmt.Sprintf("nodeB1ProxyEnabled1K%d@localhost", b.N)
	node2name := fmt.Sprintf("nodeB2ProxyEnabled1K%d@localhost", b.N)
	node3name := fmt.Sprintf("nodeB3ProxyEnabled1K%d@localhost", b.N)
	opts1 := node.Options{}
	opts1.Proxy.Flags = node.DefaultProxyFlags()
	opts1.Proxy.Flags.EnableEncryption = false
	node1, _ := ergo.StartNode(node1name, "bench", opts1)
	opts2 := node.Options{}
	opts2.Proxy.Transit = true
	node2, _ := ergo.StartNode(node2name, "bench", opts2)
	node3, _ := ergo.StartNode(node3name, "bench", node.Options{})
	defer node1.Stop()
	defer node2.Stop()
	defer node3.Stop()
	route := node.ProxyRoute{
		Proxy: node2.Name(),
	}
	node1.AddProxyRoute(node3.Name(), route)

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
	if e1 != nil {
		b.Fatal(e1)
	}
	p3, e3 := node3.Spawn("", gen.ProcessOptions{}, bgs)
	if e3 != nil {
		b.Fatal(e3)
	}

	call := makeCall{
		to:      p3.Self(),
		message: "hi",
	}
	if _, e := p1.Direct(call); e != nil {
		b.Fatal("single ping", e)
	}

	randomString := []byte(lib.RandomString(1024 * 1024))
	b.SetParallelism(15)
	b.RunParallel(func(pb *testing.PB) {
		p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
		if e1 != nil {
			b.Fatal(e1)
		}
		p3, e3 := node3.Spawn("", gen.ProcessOptions{}, bgs)
		if e3 != nil {
			b.Fatal(e3)
		}
		b.ResetTimer()
		for pb.Next() {
			call := makeCall{
				to:      p3.Self(),
				message: randomString,
			}
			_, e := p1.Direct(call)
			if e != nil {
				b.Fatal(e)
			}
		}

	})
}

func BenchmarkNodeProxy_NodeA_to_NodeC_via_NodeB_Message_1M_CompressedEncrypted(b *testing.B) {
	node1name := fmt.Sprintf("nodeB1ProxyEnabled1K%d@localhost", b.N)
	node2name := fmt.Sprintf("nodeB2ProxyEnabled1K%d@localhost", b.N)
	node3name := fmt.Sprintf("nodeB3ProxyEnabled1K%d@localhost", b.N)
	opts1 := node.Options{}
	opts1.Proxy.Flags = node.DefaultProxyFlags()
	opts1.Proxy.Flags.EnableEncryption = true
	node1, _ := ergo.StartNode(node1name, "bench", opts1)
	opts2 := node.Options{}
	opts2.Proxy.Transit = true
	node2, _ := ergo.StartNode(node2name, "bench", opts2)
	node3, _ := ergo.StartNode(node3name, "bench", node.Options{})
	defer node1.Stop()
	defer node2.Stop()
	defer node3.Stop()
	route := node.ProxyRoute{
		Proxy: node2.Name(),
	}
	node1.AddProxyRoute(node3.Name(), route)

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
	if e1 != nil {
		b.Fatal(e1)
	}
	p3, e3 := node3.Spawn("", gen.ProcessOptions{}, bgs)
	if e3 != nil {
		b.Fatal(e3)
	}

	call := makeCall{
		to:      p3.Self(),
		message: "hi",
	}
	if _, e := p1.Direct(call); e != nil {
		b.Fatal("single ping", e)
	}

	randomString := []byte(lib.RandomString(1024 * 1024))
	b.SetParallelism(15)
	b.RunParallel(func(pb *testing.PB) {
		p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
		if e1 != nil {
			b.Fatal(e1)
		}
		p1.SetCompression(true)
		p3, e3 := node3.Spawn("", gen.ProcessOptions{}, bgs)
		if e3 != nil {
			b.Fatal(e3)
		}
		b.ResetTimer()
		for pb.Next() {
			call := makeCall{
				to:      p3.Self(),
				message: randomString,
			}
			_, e := p1.Direct(call)
			if e != nil {
				b.Fatal(e)
			}
		}

	})
}

func benchCases() []benchCase {
	return []benchCase{
		{"number", 12345},
		{"string", "hello world"},
		{"tuple (PID)",
			etf.Pid{
				Node:     "node@localhost",
				ID:       1000,
				Creation: 1,
			},
		},
		{"binary 1MB", make([]byte, 1024*1024)},
	}
}
