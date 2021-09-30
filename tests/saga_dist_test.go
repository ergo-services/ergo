package test

import (
	"fmt"
	"testing"

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
	result int
}

func (gs *testSaga1) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{}
	gs.res = make(chan interface{}, 2)
	return opts, nil
}

func (gs *testSaga1) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	//task := value.(taskTX)
	//values := splitSlice(task.value, task.chunks)
	//for i := range values {
	//	//process.Next()
	//}
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

type testSagaN struct {
	gen.Saga
}

func (gs *testSagaN) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{}
	return opts, nil
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

	// stop all nodes
	node3.Stop()
	node3.Wait()
	node2.Stop()
	node2.Wait()
	node1.Stop()
	node1.Wait()
}
