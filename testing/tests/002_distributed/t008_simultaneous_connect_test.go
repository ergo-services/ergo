package distributed

import (
	"sync"
	"testing"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/gen"
)

func TestT8SimultaneousConnect(t *testing.T) {
	options1 := gen.NodeOptions{}
	options1.Network.Cookie = "123"
	options1.Log.DefaultLogger.Disable = true

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "123"
	options2.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("distT8node1@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	node2, err := ergo.StartNode("distT8node2@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	start := make(chan struct{})
	var wg sync.WaitGroup

	var err1 error
	var err2 error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, err1 = node1.Network().GetNode(node2.Name())
	}()
	go func() {
		defer wg.Done()
		<-start
		_, err2 = node2.Network().GetNode(node1.Name())
	}()

	close(start)
	wg.Wait()

	if err1 != nil {
		t.Fatal(err1)
	}
	if err2 != nil {
		t.Fatal(err2)
	}

	time.Sleep(200 * time.Millisecond)

	if len(node1.Network().Nodes()) != 1 {
		t.Fatal("node1 must have exactly one connected node")
	}
	if len(node2.Network().Nodes()) != 1 {
		t.Fatal("node2 must have exactly one connected node")
	}

	remote1, err := node1.Network().Node(node2.Name())
	if err != nil {
		t.Fatal(err)
	}
	remote2, err := node2.Network().Node(node1.Name())
	if err != nil {
		t.Fatal(err)
	}

	info1 := remote1.Info()
	info2 := remote2.Info()

	if info1.Node != node2.Name() {
		t.Fatal("incorrect remote1 node name")
	}
	if info2.Node != node1.Name() {
		t.Fatal("incorrect remote2 node name")
	}
}

func TestT8SimultaneousConnectMany(t *testing.T) {
	options1 := gen.NodeOptions{}
	options1.Network.Cookie = "123"
	options1.Log.DefaultLogger.Disable = true

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "123"
	options2.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("distT8node1many@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	node2, err := ergo.StartNode("distT8node2many@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	const parallel = 16
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, parallel*2)

	get1 := func() {
		defer wg.Done()
		<-start
		_, err := node1.Network().GetNode(node2.Name())
		results <- err
	}
	get2 := func() {
		defer wg.Done()
		<-start
		_, err := node2.Network().GetNode(node1.Name())
		results <- err
	}

	wg.Add(parallel * 2)
	for i := 0; i < parallel; i++ {
		go get1()
		go get2()
	}

	close(start)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
	close(results)

	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(200 * time.Millisecond)

	if len(node1.Network().Nodes()) != 1 {
		t.Fatal("node1 must have exactly one connected node")
	}
	if len(node2.Network().Nodes()) != 1 {
		t.Fatal("node2 must have exactly one connected node")
	}
}
