package test

import (
	"fmt"
	"math/rand"
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
	values := job.Value.([]int)
	result := sumSlice(values)
	process.SendResult(result)
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
	txs    map[gen.SagaTransactionID]int
	result int
	res    chan interface{}
}

type testSagaState struct {
	jobs map[gen.SagaJobID]gen.SagaTransactionID
}

func (gs *testSaga) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{
		Worker: &testSagaWorker{},
	}
	gs.txs = make(map[gen.SagaTransactionID]int)
	process.State = &testSagaState{jobs: make(map[gen.SagaJobID]gen.SagaTransactionID)}
	return opts, nil
}

func (gs *testSaga) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	task := value.(taskTX)
	values := splitSlice(task.value, task.chunks)
	state := process.State.(*testSagaState)
	for i := range values {
		job_id, err := process.StartJob(id, gen.SagaJobOptions{}, values[i])
		if err != nil {
			return err
		}
		state.jobs[job_id] = id
	}
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
	gs.result += result.(int)
	return gen.SagaStatusOK
}

func (gs *testSaga) HandleTxInterim(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, interim interface{}) gen.SagaStatus {

	return gen.SagaStatusOK
}

func (gs *testSaga) HandleJobResult(process *gen.SagaProcess, id gen.SagaJobID, result interface{}) gen.SagaStatus {

	state := process.State.(*testSagaState)
	tx_id := state.jobs[id]
	txValue := gs.txs[tx_id]
	txValue += result.(int)
	gs.txs[tx_id] = txValue
	delete(state.jobs, id)
	if len(state.jobs) == 0 {
		process.SendResult(tx_id, txValue)
	}
	return gen.SagaStatusOK
}

type task struct {
	value  []int
	split  int
	chunks int
}

type taskTX struct {
	value  []int
	chunks int
}

func (gs *testSaga) HandleSagaInfo(process *gen.SagaProcess, message etf.Term) gen.ServerStatus {
	fmt.Println("INFO", message)
	return gen.ServerStatusOK
}
func (gs *testSaga) HandleSagaDirect(process *gen.SagaProcess, message interface{}) (interface{}, error) {
	switch m := message.(type) {
	case task:
		values := splitSlice(m.value, m.split)
		for i := range values {
			txValue := taskTX{
				value:  values[i],
				chunks: m.chunks,
			}
			id := process.StartTransaction(gen.SagaTransactionOptions{}, txValue)
			gs.txs[id] = 0
		}

		return nil, nil
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
	slice1 := rand.Perm(10)
	sum1 := sumSlice(slice1)
	startTask := task{
		value:  slice1,
		split:  3, // split on 3 txs
		chunks: 5, // size of slice for worker
	}
	_, err = saga_process.Direct(startTask)
	if err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, saga.res, sum1)

	time.Sleep(2 * time.Second)

	node.Stop()
	node.Wait()
}

func splitSlice(slice []int, size int) [][]int {
	var chunks [][]int
	for i := 0; i < len(slice); i += size {
		end := i + size

		if end > len(slice) {
			end = len(slice)
		}

		chunks = append(chunks, slice[i:end])
	}

	return chunks

}

func sumSlice(slice []int) int {
	var result int
	for i := range slice {
		result += slice[i]
	}
	return result
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
