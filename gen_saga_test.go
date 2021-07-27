package ergo

import (
	"fmt"
	"testing"
)

type testGenSaga struct {
	GenSaga
}

func (gs *testGenSaga) InitSaga(state *GenSagaState, args ...interface{}) error {
	return nil
}

func (gs *testGenSaga) HandleNext(state *GenSagaState, tx GenSagaTransaction, value interface{}) error {
	//return "result", value
	//return "interim", value
	//return "next", []GenSagaNext
	//return "cancel", reason // cancel tx with given reason
	//return "wait", value
	//return "stop", nil // stop saga with reason 'normal'
	//return reason, nil

	return nil
}

func (gs *testGenSaga) HandleCancel(state *GenSagaState, tx GenSagaTransaction, reason string) error {
	//return "ok"
	//return "stop" // stop saga with reason 'normal'
	//return reason

	return nil
}

func (gs *testGenSaga) HandleResult(state *GenSagaState, tx GenSagaTransaction, result interface{}) error {
	//return "next", []GenSagaNext
	//return "cancel", reason // cancel tx with given reason
	//return "result", value
	//return "interim", value
	//return "stop", nil // stop saga with reason 'normal'
	//return "wait", value
	//return reason, nil

	return nil
}

func (gs *testGenSaga) HandleInterim(state *GenSagaState, tx GenSagaTransaction, interim interface{}) error {
	//return "ok"
	//return "stop" // stop saga with reason 'normal'
	//return reason

	return nil
}

func (gs *testGenSaga) HandleTimeout(state *GenSagaState, tx GenSagaTransaction, next []GenSagaNext) error {
	//return "next", []GenSagaNext
	//return "wait", value
	//return "cancel", reason // cancel tx with given reason
	//return "stop", nil      // stop saga with reason 'normal'
	//return reason, nil

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
