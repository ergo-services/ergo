package tests

import (
	"fmt"
	"testing"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/node"
)

func TestWeb(t *testing.T) {
	fmt.Printf("\n=== Test Web Server\n")
	fmt.Printf("Starting nodes: nodeWeb1@localhost: ")
	node1, _ := ergo.StartNode("nodeWeb1@localhost", "cookies", node.Options{})
	defer node1.Stop()
	if node1 == nil {
		t.Fatal("can't start nodes")
	} else {
		fmt.Println("OK")
	}
}
