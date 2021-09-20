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
	fmt.Println("Worker process started", process.Self(), " on", job.ID, " with value", job.Value)
	process.SendInterim(888)
	process.SendInterim(999)
	process.SendResult(1000)
	//panic("asdf")
	time.Sleep(500 * time.Millisecond)
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
	jobs map[gen.SagaJobID]gen.SagaTransactionID
}

func (gs *testSaga) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{
		Worker: &testSagaWorker{},
	}
	gs.jobs = make(map[gen.SagaJobID]gen.SagaTransactionID)
	return opts, nil
}

func (gs *testSaga) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	fmt.Println("Got new TX", id, value)
	job_id, err := process.StartJob(id, gen.SagaJobOptions{}, value)
	if err != nil {
		return err
	}
	fmt.Println("Started job", job_id)
	gs.jobs[job_id] = id
	return gen.SagaStatusOK
}

func (gs *testSaga) HandleTxDone(process *gen.SagaProcess, id gen.SagaTransactionID) gen.SagaStatus {
	fmt.Println("Tx done", id)
	return gen.SagaStatusOK
}

func (gs *testSaga) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSaga) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaStepID, result interface{}) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSaga) HandleTxInterim(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaStepID, interim interface{}) gen.SagaStatus {

	return gen.SagaStatusOK
}

func (gs *testSaga) HandleJobResult(process *gen.SagaProcess, id gen.SagaJobID, result interface{}) gen.SagaStatus {
	txid, _ := gs.jobs[id]
	err := process.SendResult(txid, result)
	fmt.Println("Job result", result, txid, err)
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
		fmt.Println("MMM", m.value)
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
	txID, err := saga_process.Direct(startTX{value: 555})
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("Started TX", txID)
	time.Sleep(2 * time.Second)

	node.Stop()
	node.Wait()
}
