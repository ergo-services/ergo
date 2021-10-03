package tests

import (
	"fmt"
	"testing"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
)

// this test implements cases below
// 1. Single saga cancels Tx
// 2. Saga1 -> Tx -> Saga2 -> Tx -> Saga3
//    a) Saga1 cancels Tx
//       Saga1 -> cancel -> Saga2 -> cancel -> Saga3
//    b) Saga2 cancels Tx
//       Saga1 <- cancel <- Saga2 -> cancel -> Saga3
//    c) Saga3 cancels Tx
//       Saga1 <- cancel <- Saga2 <- cancel <- Saga3
//    d) Saga1 sets TrapCancel, Saga2 process/node is going down, Saga1 sends Tx to the Saga4
//              -> Tx -> Saga4 -> Tx -> Saga3
//            /
//       Saga1 <- signal Down <- Saga2 (terminates) -> signal Down -> Saga3

//
// Case 1
//

type testSagaCancelWorker struct {
	gen.SagaWorker
}

func (w *testSagaCancelWorker) HandleJobStart(process *gen.SagaWorkerProcess, job gen.SagaJob) error {
	return nil
}
func (w *testSagaCancelWorker) HandleJobCancel(process *gen.SagaWorkerProcess) {
	return
}

type testSagaCancel struct {
	gen.Saga
}

func (gs *testSagaCancel) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{
		Worker: &testSagaCancelWorker{},
	}

	return opts, nil
}

func (gs *testSagaCancel) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSagaCancel) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSagaCancel) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSagaCancel) HandleSagaDirect(process *gen.SagaProcess, message interface{}) (interface{}, error) {
	switch m := message.(type) {
	case task:

		process.StartTransaction(gen.SagaTransactionOptions{}, 3.14)
		return nil, nil
	}

	return nil, fmt.Errorf("unknown request %#v", message)
}

func TestSagaCancelSimple(t *testing.T) {

	fmt.Printf("\n=== Test GenSagaCancelSimple\n")
	fmt.Printf("Starting node: nodeGenSagaCancelSimple01@localhost...")

	node, _ := ergo.StartNode("nodeGenSagaCancelSimple01@localhost", "cookies", node.Options{})

	if node == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")

	fmt.Printf("... Starting Saga processes: ")
	saga := &testSagaCancel{}
	saga_process, err := node.Spawn("saga", gen.ProcessOptions{MailboxSize: 10000}, saga)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	_, err = saga_process.Direct(startTask1)
	if err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, saga.res, sum1)
}

//
// Case 2.a
//    Saga1 -> Tx -> Saga2 -> Tx -> Saga3
//       Saga1 cancels Tx
//       Saga1 -> cancel -> Saga2 -> cancel -> Saga3
//

func TestSagaCancelCase2a(t *testing.T) {
	fmt.Printf("\n=== Test GenSagaCancelCase2a\n")
	fmt.Printf("Starting node: nodeGenSagaCancelCase2a01@localhost...")

	node, _ := ergo.StartNode("nodeGenSagaCancelCase2a01@localhost", "cookies", node.Options{})

	if node == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
}

//
// Case 2.b
//    Saga1 -> Tx -> Saga2 -> Tx -> Saga3
//       Saga2 cancels Tx
//       Saga1 <- cancel <- Saga2 -> cancel -> Saga3
//
func TestSagaCancelCase2b(t *testing.T) {
	fmt.Printf("\n=== Test GenSagaCancelCase2b\n")
	fmt.Printf("Starting node: nodeGenSagaCancelCase2b01@localhost...")

	node, _ := ergo.StartNode("nodeGenSagaCancelCase2b01@localhost", "cookies", node.Options{})

	if node == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
}

//
// Case 2.c
//    Saga1 -> Tx -> Saga2 -> Tx -> Saga3
//       Saga3 cancels Tx
//       Saga1 <- cancel <- Saga2 <- cancel <- Saga3
//
func TestSagaCancelCase2c(t *testing.T) {
	fmt.Printf("\n=== Test GenSagaCancelCase2c\n")
	fmt.Printf("Starting node: nodeGenSagaCancelCase2c01@localhost...")

	node, _ := ergo.StartNode("nodeGenSagaCancelCase2c01@localhost", "cookies", node.Options{})

	if node == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
}

//
// Case 2.d
//    Saga1 -> Tx -> Saga2 -> Tx -> Saga3
//       Saga1 sets TrapCancel, Saga2 process/node is going down, Saga1 sends Tx to the Saga4
//              -> Tx -> Saga4 -> Tx -> Saga3
//            /
//       Saga1 <- signal Down <- Saga2 (terminates) -> signal Down -> Saga3
//
func TestSagaCancelCase2d(t *testing.T) {
	fmt.Printf("\n=== Test GenSagaCancelCase2d\n")
	fmt.Printf("Starting node: nodeGenSagaCancelCase2d01@localhost...")

	node, _ := ergo.StartNode("nodeGenSagaCancelCase2d01@localhost", "cookies", node.Options{})

	if node == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
}
