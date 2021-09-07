package test

import (
	"fmt"
	"testing"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
)

type testSaga struct {
	gen.Saga
}

func (gs *testGenSaga) InitSaga(state *gen.SagaState, args ...etf.Term) error {
	return nil
}

func (gs *testGenSaga) HandleNext(state *gen.SagaState, tx gen.SagaTransaction, value interface{}) error {
	//return "result", value
	//return "interim", value
	//return "next", []gen.SagaNext
	//return "cancel", reason // cancel tx with given reason
	//return "stop", nil // stop saga with reason 'normal'
	//return "noreply", nil
	//return reason, nil

	return nil
}

func (gs *testGenSaga) HandleCancel(state *gen.SagaState, tx gen.SagaTransaction, reason string) error {
	//return "ok"
	//return "stop" // stop saga with reason 'normal'
	//return reason

	return nil
}

func (gs *testGenSaga) HandleResult(state *gen.SagaState, tx gen.SagaTransaction, from gen.SagaNext, result interface{}) error {
	//return "next", []gen.SagaNext
	//return "cancel", reason // cancel tx with given reason
	//return "result", value
	//return "interim", value
	//return "stop", nil // stop saga with reason 'normal'
	//return "wait", value
	//return reason, nil

	return nil
}

func (gs *testGenSaga) HandleInterim(state *gen.SagaState, tx gen.SagaTransaction, from gen.SagaNext, interim interface{}) error {
	//return "ok"
	//return "stop" // stop saga with reason 'normal'
	//return reason

	return nil
}

func (gs *testGenSaga) HandleTimeout(state *gen.SagaState, tx gen.SagaTransaction, from gen.SagaNext) error {
	//return "next", []gen.SagaNext
	//return "wait", value
	//return "cancel", reason // cancel tx with given reason
	//return "stop", nil      // stop saga with reason 'normal'
	//return reason, nil

	return nil
}

type testGenSagaWorker struct {
	gen.SagaWorker
}

func (w *testGenSagaWorker) HandleStartJob(state *gen.SagaWorkerState) error {
	return nil
}
func (w *testGenSagaWorker) HandleCancelJob(state *gen.SagaWorkerState) {
	return
}

func TestGenSagaSimple(t *testing.T) {
	fmt.Printf("\n=== Test GenSagaSimple\n")
	fmt.Printf("Starting node: nodeGenSagaSimple01@localhost...")

	node, _ := ergo.StartNode("nodeGenSagaSimple01@localhost", "cookies", node.Options{})

	if node == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")

	fmt.Printf("... starting Saga processes: ")
	saga := &testGenSaga{}
	saga_opts := gen.SagaOptions{
		Worker: &testGenSagaWorker{},
	}
	saga_process, err := node.Spawn("saga", ProcessOptions{}, saga, saga_opts)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK", saga_process.Self())

	node.Stop()
}
