package ergo

import (
	"fmt"
	"testing"
)

type testGenSaga struct {
	GenSaga
}

func (gs *testGenSaga) InitSaga(p *Process, args ...interface{}) (GenSagaOptions, interface{}) {
	opts := GenSagaOptions{}
	return opts, nil
}

func (gs *testGenSaga) HandleCancel(tx GenSagaTransaction, state GenSagaState) error {
	return nil
}

func (gs *testGenSaga) HandleCanceled(tx GenSagaTransaction, reason string, state GenSagaState) error {
	return nil
}

func (gs *testGenSaga) HandleDone(tx GenSagaTransaction, result interface{}, state GenSagaState) error {
	return nil
}

func (gs *testGenSaga) HandleTimeout(tx GenSagaTransaction, timeout int, state GenSagaState) error {
	return nil
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
