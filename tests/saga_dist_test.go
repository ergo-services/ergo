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

// this test implemets distributed case of computing sum for the given
// slice of int numbers
//
//           -> Saga2 (run workers 1..n)
//         /
// Saga1 ->
//         \
//           -> Saga3 (run workers 1..n)
//
// Saga1 creates a slice of int numbers, split it and sends to the Saga2 and Saga3.
// Each saga runs on a separated node.

//
// Saga1
//
type testSaga1 struct {
	gen.Saga
	res    chan interface{}
	result int
}

type testSaga1State struct {
	txs map[gen.SagaTransactionID]*txvalue
}
type txvalue struct {
	res   int
	nexts map[gen.SagaNextID]bool
}

func (gs *testSaga1) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{}
	gs.res = make(chan interface{}, 2)
	process.State = &testSaga1State{
		txs: make(map[gen.SagaTransactionID]*txvalue),
	}
	return opts, nil
}

func (gs *testSaga1) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	state := process.State.(*testSaga1State)
	task := value.(taskTX)

	txval := &txvalue{
		nexts: make(map[gen.SagaNextID]bool),
	}
	// split it into two parts (two sagas)
	values := splitSlice(task.value, len(task.value)/2+1)

	// send to the saga2
	next := gen.SagaNext{
		Saga:  saga2_process,
		Value: values[0],
	}
	next_id, err := process.Next(id, next)
	if err != nil {
		panic(err)
	}
	txval.nexts[next_id] = true

	// send to the saga3
	if len(values) > 1 {
		next = gen.SagaNext{
			Saga:  saga3_process,
			Value: values[1],
		}
		next_id, err := process.Next(id, next)
		if err != nil {
			panic(err)
		}
		txval.nexts[next_id] = true
	}

	state.txs[id] = txval
	return gen.SagaStatusOK
}

func (gs *testSaga1) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSaga1) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
	state := process.State.(*testSaga1State)
	txval := state.txs[id]
	switch r := result.(type) {
	case int:
		txval.res += r
	case int64:
		txval.res += int(r)
	}
	delete(txval.nexts, from)
	if len(txval.nexts) == 0 {
		process.SendResult(id, txval.res)
	}
	return gen.SagaStatusOK
}

func (gs *testSaga1) HandleTxDone(process *gen.SagaProcess, id gen.SagaTransactionID, result interface{}) (interface{}, gen.SagaStatus) {
	state := process.State.(*testSaga1State)
	txval := state.txs[id]
	delete(state.txs, id)
	gs.result += txval.res
	if len(state.txs) == 0 {
		gs.res <- gs.result
	}

	return nil, gen.SagaStatusOK
}
func (gs *testSaga1) HandleSagaDirect(process *gen.SagaProcess, message interface{}) (interface{}, error) {
	switch m := message.(type) {
	case task:
		values := splitSlice(m.value, m.split)
		fmt.Printf("    process %v txs with %v value(s) each distributing them on Saga2 and Saga3: ", len(values), m.split)
		for i := range values {
			txValue := taskTX{
				value:  values[i],
				chunks: m.chunks,
			}
			process.StartTransaction(gen.SagaTransactionOptions{}, txValue)
		}

		return nil, nil
	}

	return nil, fmt.Errorf("unknown request %#v", message)
}

//
// Saga2/Saga3
//

var (
	saga2_process = gen.ProcessID{
		Name: "saga2",
		Node: "nodeGenSagaDist02@localhost",
	}

	saga3_process = gen.ProcessID{
		Name: "saga3",
		Node: "nodeGenSagaDist03@localhost",
	}
)

type testSagaN struct {
	gen.Saga
}

type testSagaNState struct {
	txs map[gen.SagaTransactionID]*txjobs
}

func (gs *testSagaN) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{
		Worker: &testSagaWorkerN{},
	}
	process.State = &testSagaState{
		txs: make(map[gen.SagaTransactionID]*txjobs),
	}
	return opts, nil
}
func (gs *testSagaN) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	var vv []int
	if err := etf.TermIntoStruct(value, &vv); err != nil {
		panic(err)
	}
	state := process.State.(*testSagaState)
	j := txjobs{
		jobs: make(map[gen.SagaJobID]bool),
	}

	values := splitSlice(vv, 5)
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

func (gs *testSagaN) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSagaN) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSagaN) HandleJobResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaJobID, result interface{}) gen.SagaStatus {
	state := process.State.(*testSagaState)
	j := state.txs[id]
	j.result += result.(int)
	delete(j.jobs, from)

	if len(j.jobs) == 0 {
		process.SendResult(id, j.result)
	}

	return gen.SagaStatusOK
}

//
// SagaWorkerN
//
type testSagaWorkerN struct {
	gen.SagaWorker
}

func (w *testSagaWorkerN) HandleJobStart(process *gen.SagaWorkerProcess, job gen.SagaJob) error {
	values := job.Value.([]int)
	result := sumSlice(values)
	if err := process.SendResult(result); err != nil {
		panic(err)
	}
	return nil
}

func (w *testSagaWorkerN) HandleJobCancel(process *gen.SagaWorkerProcess, reason string) {
	return
}

func TestSagaDist(t *testing.T) {
	fmt.Printf("\n=== Test GenSagaDist\n")

	fmt.Printf("Starting node: nodeGenSagaDist01@localhost...")
	node1, _ := ergo.StartNode("nodeGenSagaDist01@localhost", "cookies", node.Options{})
	if node1 == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
	fmt.Printf("Starting node: nodeGenSagaDist02@localhost...")
	node2, _ := ergo.StartNode("nodeGenSagaDist02@localhost", "cookies", node.Options{})
	if node2 == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
	fmt.Printf("Starting node: nodeGenSagaDist03@localhost...")
	node3, _ := ergo.StartNode("nodeGenSagaDist03@localhost", "cookies", node.Options{})
	if node3 == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")

	fmt.Printf("... Starting Saga1 processes (on node1): ")
	saga1 := &testSaga1{}
	saga1_process, err := node1.Spawn("saga1", gen.ProcessOptions{MailboxSize: 10000}, saga1)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... Starting Saga2 processes (on node2): ")
	saga2 := &testSagaN{}
	_, err = node2.Spawn("saga2", gen.ProcessOptions{MailboxSize: 10000}, saga2)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... Starting Saga3 processes (on node3): ")
	saga3 := &testSagaN{}
	_, err = node3.Spawn("saga3", gen.ProcessOptions{MailboxSize: 10000}, saga3)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	rand.Seed(time.Now().Unix())

	slice1 := rand.Perm(1000)
	sum1 := sumSlice(slice1)
	saga1.result = 0
	startTask1 := task{
		value: slice1,
		split: 1,
	}
	saga1_process.Direct(startTask1)
	waitForResultWithValue(t, saga1.res, sum1)

	slice2 := rand.Perm(1000)
	sum2 := sumSlice(slice2)
	saga1.result = 0
	startTask2 := task{
		value: slice2,
		split: 100,
	}
	saga1_process.Direct(startTask2)
	waitForResultWithValue(t, saga1.res, sum2)

	slice3 := rand.Perm(1000)
	sum3 := sumSlice(slice3)
	saga1.result = 0
	startTask3 := task{
		value: slice3,
		split: 1000,
	}
	saga1_process.Direct(startTask3)
	waitForResultWithValue(t, saga1.res, sum3)

	// stop all nodes
	node3.Stop()
	node3.Wait()
	node2.Stop()
	node2.Wait()
	node1.Stop()
	node1.Wait()
}
