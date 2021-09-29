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

// -----------------------
// ----- Saga Simple -----

//
// Worker
//
type testSagaWorker struct {
	gen.SagaWorker
}

func (w *testSagaWorker) HandleJobStart(process *gen.SagaWorkerProcess, job gen.SagaJob) error {
	fmt.Println("... Worker process started", process.Self(), " on", job.ID, " with value", job.Value, "for TX", job.TransactionID)
	process.SendInterim(888)
	process.SendInterim(999)
	process.SendResult(1000)
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
	job_id, err := process.StartJob(id, gen.SagaJobOptions{}, value)
	if err != nil {
		return err
	}
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

func (gs *testSaga) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
	fmt.Println("Tx result", id, result)
	return gen.SagaStatusOK
}

func (gs *testSaga) HandleTxInterim(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, interim interface{}) gen.SagaStatus {

	return gen.SagaStatusOK
}

func (gs *testSaga) HandleJobInterim(process *gen.SagaProcess, id gen.SagaJobID, interim interface{}) gen.SagaStatus {
	fmt.Println("... Saga got interim result", interim, "from job", id)
	return gen.SagaStatusOK
}
func (gs *testSaga) HandleJobResult(process *gen.SagaProcess, id gen.SagaJobID, result interface{}) gen.SagaStatus {
	txid, _ := gs.jobs[id]
	if err := process.SendResult(txid, result); err != nil {
		panic(err)
	}
	fmt.Println("... Saga got job result", result, "from job", id)
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

	fmt.Printf("... Starting Saga processes: ")
	saga := &testSaga{}
	saga_process, err := node.Spawn("saga", gen.ProcessOptions{}, saga)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... Starting new TX with initial value 12345 ")
	txID, err := saga_process.Direct(startTX{value: 12345})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("and txid", txID)

	time.Sleep(2 * time.Second)

	node.Stop()
	node.Wait()
}

// ----- Saga Simple -----
// -----------------------

// ----------------------------
// ----- Saga Distributed -----

type testSagaDist struct {
	gen.Saga
}

// ----- Saga Distributed -----
// ----------------------------
