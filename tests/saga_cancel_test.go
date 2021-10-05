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

type taskSagaCancelCase1 struct {
	workerRes chan interface{}
	sagaRes   chan interface{}
}

type testSagaCancelWorker struct {
	gen.SagaWorker
}

func (w *testSagaCancelWorker) HandleJobStart(process *gen.SagaWorkerProcess, job gen.SagaJob) error {
	process.State = job.Value
	return nil
}
func (w *testSagaCancelWorker) HandleJobCancel(process *gen.SagaWorkerProcess, reason string) {
	if err := process.SendInterim(1); err != gen.ErrSagaTxCanceled {
		panic("shouldn't be able to send interim result")
	}
	if err := process.SendResult(1); err != gen.ErrSagaTxCanceled {
		panic("shouldn't be able to send the result")
	}
	task := process.State.(taskSagaCancelCase1)
	task.workerRes <- "ok"
	return
}

type testSagaCancel struct {
	gen.Saga
}

func (gs *testSagaCancel) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	worker := &testSagaCancelWorker{}
	opts := gen.SagaOptions{
		Worker: worker,
	}
	return opts, nil
}

func (gs *testSagaCancel) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	process.State = value
	task := process.State.(taskSagaCancelCase1)
	task.sagaRes <- "startTX"

	_, err := process.StartJob(id, gen.SagaJobOptions{}, value)
	if err != nil {
		panic(err)
	}
	task.workerRes <- "startWorker"
	if err := process.CancelTransaction(id, "test cancel"); err != nil {
		panic(err)
	}

	// try to cancel unknown TX
	if err := process.CancelTransaction(gen.SagaTransactionID{}, "bla bla"); err != gen.ErrSagaTxUnknown {
		panic("must be ErrSagaTxUnknown")
	}
	task.sagaRes <- "cancelTX"
	return gen.SagaStatusOK
}

func (gs *testSagaCancel) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	task := process.State.(taskSagaCancelCase1)
	if reason == "test cancel" {
		task.sagaRes <- "ok"
	}
	return gen.SagaStatusOK
}

func (gs *testSagaCancel) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSagaCancel) HandleSagaDirect(process *gen.SagaProcess, message interface{}) (interface{}, error) {

	process.StartTransaction(gen.SagaTransactionOptions{}, message)
	return nil, nil
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

	task := taskSagaCancelCase1{
		workerRes: make(chan interface{}, 2),
		sagaRes:   make(chan interface{}, 2),
	}
	_, err = saga_process.Direct(task)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("... Start new TX on saga: ")
	waitForResultWithValue(t, task.sagaRes, "startTX")
	fmt.Printf("... Start new worker on saga: ")
	waitForResultWithValue(t, task.workerRes, "startWorker")
	fmt.Printf("... Cancel TX on saga: ")
	waitForResultWithValue(t, task.sagaRes, "cancelTX")
	fmt.Printf("... Saga worker handled TX cancelation: ")
	waitForResultWithValue(t, task.workerRes, "ok")
	fmt.Printf("... Saga handled TX cancelation: ")
	waitForResultWithValue(t, task.sagaRes, "ok")
}

/*

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
*/
