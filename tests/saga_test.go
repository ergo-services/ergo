package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
)

//
// Worker
//
type testSagaWorker struct {
	gen.SagaWorker
}

func (w *testSagaWorker) HandleJobStart(process *gen.SagaWorkerProcess, job gen.SagaJob) error {
	return nil
}
func (w *testSagaWorker) HandleJobCancel(process *gen.SagaWorkerProcess) {
	return
}
func (w *testSagaWorker) HandleWorkerInfo(process *gen.SagaWorkerProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}

//
// Saga
//
type testSaga struct {
	gen.Saga
}

func (gs *testSaga) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{
		Worker: &testSagaWorker{},
	}
	return opts, nil
}

func (gs *testSaga) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	fmt.Println("Got new TX", id)
	return gen.SagaStatusOK
}

func (gs *testSaga) HandleTxDone(process *gen.SagaProcess, id gen.SagaTransactionID) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSaga) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSaga) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSaga) HandleTxInterim(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, interim interface{}) gen.SagaStatus {

	return gen.SagaStatusOK
}

type startTX struct {
	name  string
	opts  gen.SagaTransactionOptions
	value interface{}
}

func (gs *testSaga) HandleSagaInfo(process *gen.SagaProcess, message etf.Term) gen.ServerStatus {
	fmt.Println("INFO", message)
	return gen.ServerStatusOK
}
func (gs *testSaga) HandleSagaDirect(process *gen.SagaProcess, message interface{}) (interface{}, error) {
	switch m := message.(type) {
	case startTX:
		id := process.StartTransaction(m.name, m.opts, m.value)
		return id, nil
	}

	return nil, fmt.Errorf("unknown request %#v", message)
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
	txID, err := saga_process.Direct(startTX{})
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("Stated TX", txID)
	time.Sleep(1 * time.Second)

	node.Stop()
}
