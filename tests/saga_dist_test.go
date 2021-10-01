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
	sw     bool
	result int
}

func (gs *testSaga1) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{}
	gs.res = make(chan interface{}, 2)
	return opts, nil
}

func (gs *testSaga1) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	task := value.(taskTX)
	values := splitSlice(task.value, task.chunks)
	for i := range values {
		saga := saga2_process
		gs.sw = !gs.sw
		if gs.sw {
			saga = saga3_process
		}
		next := gen.SagaNext{
			Saga:  saga,
			Value: values[i],
		}
		next_id, err := process.Next(id, next)
		if err != nil {
			panic(err)
		}
		fmt.Println("send to", next, "next_id", next_id)
	}
	return gen.SagaStatusOK
}

func (gs *testSaga1) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSaga1) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
	fmt.Println("got result from", from, "value", result)
	gs.result += result.(int)
	return gen.SagaStatusOK
}

func (gs *testSaga1) HandleSagaDirect(process *gen.SagaProcess, message interface{}) (interface{}, error) {
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

func (gs *testSagaN) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{
		Worker: &testSagaWorkerN{},
	}
	return opts, nil
}
func (gs *testSagaN) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	fmt.Println(process.Name(), "got new tx", id, "with value", value)
	var vv []int
	if err := etf.TermIntoStruct(value, &vv); err != nil {
		panic(err)
	}
	values := splitSlice(vv, 5)
	for i := range values {
		_, err := process.StartJob(id, gen.SagaJobOptions{}, values[i])
		if err != nil {
			return err
		}
	}
	return gen.SagaStatusOK
}

func (gs *testSagaN) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSagaN) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
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

func (w *testSagaWorkerN) HandleJobCancel(process *gen.SagaWorkerProcess) {
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
	startTask1 := task{
		value:  slice1,
		split:  1,
		chunks: 5,
	}
	saga1_process.Direct(startTask1)

	time.Sleep(time.Second)

	// stop all nodes
	node3.Stop()
	node3.Wait()
	node2.Stop()
	node2.Wait()
	node1.Stop()
	node1.Wait()
}
