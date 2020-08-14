package ergo

import (
	"fmt"
	"testing"
)

type GenStageTest struct {
	GenStage
}

func TestGenStage(t *testing.T) {
	fmt.Printf("\n=== Test GenStage\n")
	fmt.Printf("Starting node: nodeGenStage01@localhost...")

	node1 := CreateNode("nodeGenStage01@localhost", "cookies", NodeOptions{})

	if node1 == nil {
		t.Fatal("can't start node")
		return
	}

	fmt.Println("OK")

	node1.Stop()
}
