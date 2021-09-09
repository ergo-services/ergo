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

type testSagaWorker struct {
	gen.SagaWorker
}

func (w *testSagaWorker) HandleStartJob(process *gen.SagaWorkerProcess) error {
	return nil
}
func (w *testSagaWorker) HandleCancelJob(process *gen.SagaWorkerProcess) {
	return
}

func (gs *testSaga) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{
		Worker: &testSagaWorker{},
	}
	return opts, nil
}

func (gs *testSaga) HandleNext(process *gen.SagaProcess, tx gen.SagaTransaction, value interface{}) error {
	//return "result", value
	//return "interim", value
	//return "next", []gen.SagaNext
	//return "cancel", reason // cancel tx with given reason
	//return "stop", nil // stop saga with reason 'normal'
	//return "noreply", nil
	//return reason, nil

	return nil
}

func (gs *testSaga) HandleCancel(process *gen.SagaProcess, tx gen.SagaTransaction, reason string) error {
	//return "ok"
	//return "stop" // stop saga with reason 'normal'
	//return reason

	return nil
}

func (gs *testSaga) HandleResult(process *gen.SagaProcess, tx gen.SagaTransaction, from gen.SagaNext, result interface{}) error {
	//return "next", []gen.SagaNext
	//return "cancel", reason // cancel tx with given reason
	//return "result", value
	//return "interim", value
	//return "stop", nil // stop saga with reason 'normal'
	//return "wait", value
	//return reason, nil

	return nil
}

func (gs *testSaga) HandleInterim(process *gen.SagaProcess, tx gen.SagaTransaction, from gen.SagaNext, interim interface{}) error {
	//return "ok"
	//return "stop" // stop saga with reason 'normal'
	//return reason

	return nil
}

func (gs *testSaga) HandleTimeout(process *gen.SagaProcess, tx gen.SagaTransaction, from gen.SagaNext) error {
	//return "next", []gen.SagaNext
	//return "wait", value
	//return "cancel", reason // cancel tx with given reason
	//return "stop", nil      // stop saga with reason 'normal'
	//return reason, nil

	return nil
}

func TestSagaSimple(t *testing.T) {
	fmt.Printf("\n=== Test GenSagaSimple\n")
	fmt.Printf("Starting node: nodeGenSagaSimple01@localhost...")

	node, _ := ergo.StartNode("nodeGenSagaSimple01@localhost", "cookies", node.Options{})

	if node == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")

	fmt.Printf("... starting Saga processes: ")
	saga := &testSaga{}
	saga_process, err := node.Spawn("saga", gen.ProcessOptions{}, saga)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK", saga_process.Self())

	node.Stop()
}
