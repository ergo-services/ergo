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

type testSagaCancelWorker1 struct {
	gen.SagaWorker
}

func (w *testSagaCancelWorker1) HandleJobStart(process *gen.SagaWorkerProcess, job gen.SagaJob) error {
	process.State = job.Value
	return nil
}
func (w *testSagaCancelWorker1) HandleJobCancel(process *gen.SagaWorkerProcess, reason string) {
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

type testSagaCancel1 struct {
	gen.Saga
}

func (gs *testSagaCancel1) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	worker := &testSagaCancelWorker1{}
	opts := gen.SagaOptions{
		Worker: worker,
	}
	return opts, nil
}

func (gs *testSagaCancel1) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
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

func (gs *testSagaCancel1) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	task := process.State.(taskSagaCancelCase1)
	if reason == "test cancel" {
		task.sagaRes <- "ok"
	}
	return gen.SagaStatusOK
}

func (gs *testSagaCancel1) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSagaCancel1) HandleSagaDirect(process *gen.SagaProcess, message interface{}) (interface{}, error) {

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
	defer node.Stop()

	fmt.Printf("... Starting Saga processes: ")
	saga := &testSagaCancel1{}
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

//
// Case 2.a
//    Node1.Saga1 -> Tx -> Node2.Saga2 -> Tx -> Node3.Saga3
//       Node1.Saga1 cancels Tx
//       Node1.Saga1 -> cancel -> Node2.Saga2 -> cancel -> Node3Saga3
//

type testSagaCancelWorker2 struct {
	gen.SagaWorker
}

func (w *testSagaCancelWorker2) HandleJobStart(process *gen.SagaWorkerProcess, job gen.SagaJob) error {
	process.State = job.Value
	return nil
}
func (w *testSagaCancelWorker2) HandleJobCancel(process *gen.SagaWorkerProcess, reason string) {
	if err := process.SendInterim(1); err != gen.ErrSagaTxCanceled {
		panic("shouldn't be able to send interim result")
	}
	if err := process.SendResult(1); err != gen.ErrSagaTxCanceled {
		panic("shouldn't be able to send the result")
	}
	args := process.State.(testSagaCancel2Args)
	args.workerRes <- "ok"
	return
}

type testSagaCancel2 struct {
	gen.Saga
}

type testSagaCancel2Args struct {
	workerRes chan interface{}
	sagaRes   chan interface{}
	testCase  int // 1,2,3,4
}

func (gs *testSagaCancel2) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	worker := &testSagaCancelWorker2{}
	opts := gen.SagaOptions{
		Worker: worker,
	}
	process.State = args[0] // testSagaCancel2Args
	return opts, nil
}

func (gs *testSagaCancel2) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	args := process.State.(testSagaCancel2Args)
	args.sagaRes <- "startTX"

	_, err := process.StartJob(id, gen.SagaJobOptions{}, process.State)
	if err != nil {
		panic(err)
	}
	args.workerRes <- "startWorker"
	if err := process.CancelTransaction(id, "test cancel"); err != nil {
		panic(err)
	}

	next := gen.SagaNext{}
	switch process.Name() {
	case "saga1":
		next.Saga = gen.ProcessID{Name: "saga2", Node: "node2GenSagaCancelCase2@localhost"}
	case "saga2":
		next.Saga = gen.ProcessID{Name: "saga3", Node: "node3GenSagaCancelCase2@localhost"}
	default:
		return gen.SagaStatusOK
	}
	if args.testCase == 4 {
		next.TrapCancel = true
	}
	process.Next(id, next)

	return gen.SagaStatusOK
}

func (gs *testSagaCancel2) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	args := process.State.(testSagaCancel2Args)
	if reason == "test cancel" {
		args.sagaRes <- "ok"
	}
	return gen.SagaStatusOK
}

func (gs *testSagaCancel2) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSagaCancel2) HandleSagaDirect(process *gen.SagaProcess, message interface{}) (interface{}, error) {

	process.StartTransaction(gen.SagaTransactionOptions{}, message)
	return nil, nil
}

func TestSagaCancelCase2a(t *testing.T) {
	fmt.Printf("\n=== Test GenSagaCancelCase2\n")

	fmt.Printf("Starting node: node1GenSagaCancelCase2@localhost...")
	node1, _ := ergo.StartNode("node1GenSagaCancelCase2@localhost", "cookies", node.Options{})

	if node1 == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
	defer node1.Stop()

	fmt.Printf("Starting node: node2GenSagaCancelCase2@localhost...")
	node2, _ := ergo.StartNode("node2GenSagaCancelCase2@localhost", "cookies", node.Options{})

	if node2 == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
	defer node2.Stop()

	fmt.Printf("Starting node: node3GenSagaCancelCase2@localhost...")
	node3, _ := ergo.StartNode("node3GenSagaCancelCase2@localhost", "cookies", node.Options{})

	if node3 == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
	defer node3.Stop()

	args1 := testSagaCancel2Args{
		workerRes: make(chan interface{}, 2),
		sagaRes:   make(chan interface{}, 2),
		testCase:  1,
	}

	fmt.Printf("Starting saga1 on node1GenSagaCancelCase2@localhost...")
	saga1 := &testSagaCancel2{}
	saga1_process, err := node1.Spawn("saga1", gen.ProcessOptions{MailboxSize: 10000}, saga1, args1)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	args2 := testSagaCancel2Args{
		workerRes: make(chan interface{}, 2),
		sagaRes:   make(chan interface{}, 2),
		testCase:  1,
	}
	fmt.Printf("Starting saga2 on node2GenSagaCancelCase2@localhost...")
	saga2 := &testSagaCancel2{}
	saga2_process, err := node2.Spawn("saga2", gen.ProcessOptions{MailboxSize: 10000}, saga2, args2)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	args3 := testSagaCancel2Args{
		workerRes: make(chan interface{}, 2),
		sagaRes:   make(chan interface{}, 2),
		testCase:  1,
	}
	fmt.Printf("Starting saga3 on node3GenSagaCancelCase2@localhost...")
	saga3 := &testSagaCancel2{}
	saga3_process, err := node3.Spawn("saga3", gen.ProcessOptions{MailboxSize: 10000}, saga3, args3)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK", saga2_process.Self(), saga3_process.Self())

	_, err = saga1_process.Direct(nil)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("    Case a: Node1.Saga1 -> cancel -> Node2.Saga2 -> cancel -> Node3Saga3")
	fmt.Printf("... Start new TX on saga1: ")
	waitForResultWithValue(t, args1.sagaRes, "startTX")
	fmt.Printf("... Start new worker on saga1: ")
	waitForResultWithValue(t, args1.workerRes, "startWorker")

	fmt.Printf("... Start new TX on saga2: ")
	waitForResultWithValue(t, args2.sagaRes, "startTX")
	fmt.Printf("... Start new worker on saga2: ")
	waitForResultWithValue(t, args2.workerRes, "startWorker")

	fmt.Printf("... Start new TX on saga3: ")
	waitForResultWithValue(t, args3.sagaRes, "startTX")
	fmt.Printf("... Start new worker on saga3: ")
	waitForResultWithValue(t, args3.workerRes, "startWorker")
	//
	// Case 2.d
	//    Node1.Saga1 -> Tx -> Node2.Saga2 -> Tx -> Node3.Saga3
	//    d) Saga1 sets TrapCancel, Saga2 process/node is going down, Saga1 sends Tx to the Saga4
	//              -> Tx -> Saga4 -> Tx -> Saga3
	//            /
	//       Saga1 <- signal Down <- Saga2 (terminates) -> signal Down -> Saga3
	//

	/*
		fmt.Println("      case d: Saga1 sets TrapCancel, Saga2 process/node is going down, Saga1 sends Tx to the Saga4")
		fmt.Println("        -> Tx -> Saga4 -> Tx -> Saga3")
		fmt.Println("      /")
		fmt.Println(" Saga1 <- signal Down <- Saga2 (terminates) -> signal Down -> Saga3")
		fmt.Printf("Starting node: node4GenSagaCancelCase2@localhost...")
		node4, _ := ergo.StartNode("node4GenSagaCancelCase2@localhost", "cookies", node.Options{})

		if node4 == nil {
			t.Fatal("can't start node")
			return
		}
		fmt.Println("OK")
		defer node4.Stop()

		fmt.Printf("Starting saga4 on node4GenSagaCancelCase2@localhost...")
		saga4 := &testSagaCancel2{}
		saga4_process, err := node4.Spawn("saga4", gen.ProcessOptions{MailboxSize: 10000}, saga4, args)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println("OK", saga4_process.Self())
	*/
}

/*

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
