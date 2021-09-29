package test

import (
	"fmt"
	"testing"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/node"
)

func TestSagaDist(t *testing.T) {
	fmt.Printf("\n=== Test GenSagaDist\n")
	fmt.Printf("Starting node: nodeGenSagaDist@localhost...")

	node, _ := ergo.StartNode("nodeGenSagaDist@localhost", "cookies", node.Options{})

	if node == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")

	node.Stop()
	node.Wait()
}
