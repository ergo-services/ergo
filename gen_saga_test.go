package ergo

import (
	"fmt"
	"testing"
)

type testGenSaga struct {
	GenSaga
}

func TestGenSagaSimple(t *testing.T) {
	fmt.Printf("\n=== Test GenSagaSimple\n")
	fmt.Printf("Starting node: nodeGenSagaSimple01@localhost...")

	node := CreateNode("nodeGenSagaSimple01@localhost", "cookies", NodeOptions{})

	if node == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")

	fmt.Printf("... starting Saga processes: ")
	saga := &testGenSaga{}
	saga_process, err := node.Spawn("saga", ProcessOptions{}, saga, nil)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK", saga_process.Self())

	node.Stop()
}
