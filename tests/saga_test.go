package tests

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

// Worker
type testSagaWorker struct {
	gen.SagaWorker
}

func (w *testSagaWorker) HandleJobStart(process *gen.SagaWorkerProcess, job gen.SagaJob) error {
	values := job.Value.([]int)
	result := sumSlice(values)
	if err := process.SendResult(result); err != nil {
		panic(err)
	}
	return nil
}
func (w *testSagaWorker) HandleJobCancel(process *gen.SagaWorkerProcess, reason string) {
	return
}

// Saga
type testSaga struct {
	gen.Saga
	res    chan interface{}
	result int
}

type testSagaState struct {
	txs map[gen.SagaTransactionID]*txjobs
}

type txjobs struct {
	result int
	jobs   map[gen.SagaJobID]bool
}

func (gs *testSaga) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{
		Worker: &testSagaWorker{},
	}
	gs.res = make(chan interface{}, 2)
	process.State = &testSagaState{
		txs: make(map[gen.SagaTransactionID]*txjobs),
	}
	return opts, nil
}

func (gs *testSaga) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	task := value.(taskTX)
	values := splitSlice(task.value, task.chunks)
	state := process.State.(*testSagaState)
	j := txjobs{
		jobs: make(map[gen.SagaJobID]bool),
	}
	for i := range values {
		job_id, err := process.StartJob(id, gen.SagaJobOptions{}, values[i])
		if err != nil {
			return err
		}
		j.jobs[job_id] = true
	}
	state.txs[id] = &j
	return gen.SagaStatusOK
}

func (gs *testSaga) HandleTxDone(process *gen.SagaProcess, id gen.SagaTransactionID, result interface{}) (interface{}, gen.SagaStatus) {
	state := process.State.(*testSagaState)

	gs.result += result.(int)
	delete(state.txs, id)
	if len(state.txs) == 0 {
		gs.res <- gs.result
	}
	return nil, gen.SagaStatusOK
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

func (gs *testSaga) HandleJobResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaJobID, result interface{}) gen.SagaStatus {
	state := process.State.(*testSagaState)
	j := state.txs[id]
	j.result += result.(int)
	delete(j.jobs, from)

	if len(j.jobs) == 0 {
		process.SendResult(id, j.result)
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

func (gs *testSaga) HandleSagaDirect(process *gen.SagaProcess, ref etf.Ref, message interface{}) (interface{}, gen.DirectStatus) {
	switch m := message.(type) {
	case task:
		values := splitSlice(m.value, m.split)
		fmt.Printf("    process %v txs with %v value(s) each and chunk size %v: ", len(values), m.split, m.chunks)
		for i := range values {
			txValue := taskTX{
				value:  values[i],
				chunks: m.chunks,
			}
			process.StartTransaction(gen.SagaTransactionOptions{}, txValue)
		}

		return nil, gen.DirectStatusOK
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
	saga_process, err := node.Spawn("saga", gen.ProcessOptions{MailboxSize: 10000}, saga)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	rand.Seed(time.Now().Unix())

	slice1 := rand.Perm(1000)
	sum1 := sumSlice(slice1)
	startTask1 := task{
		value:  slice1,
		split:  10, // 10 items per tx
		chunks: 5,  // size of slice for worker
	}
	_, err = saga_process.Direct(startTask1)
	if err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, saga.res, sum1)

	saga.result = 0
	slice2 := rand.Perm(100)
	sum2 := sumSlice(slice2)
	startTask2 := task{
		value:  slice2,
		split:  1, // 1 items per tx
		chunks: 5, // size of slice for worker
	}
	_, err = saga_process.Direct(startTask2)
	if err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, saga.res, sum2)

	saga.result = 0
	slice3 := rand.Perm(100)
	sum3 := sumSlice(slice3)
	startTask3 := task{
		value:  slice3,
		split:  100, // 100 items per tx
		chunks: 5,   // size of slice for worker
	}
	_, err = saga_process.Direct(startTask3)
	if err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, saga.res, sum3)

	saga.result = 0
	slice4 := rand.Perm(10000)
	sum4 := sumSlice(slice4)
	startTask4 := task{
		value:  slice4,
		split:  100, // 100 items per tx
		chunks: 5,   // size of slice for worker
	}
	_, err = saga_process.Direct(startTask4)
	if err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, saga.res, sum4)
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
