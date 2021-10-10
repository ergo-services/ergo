package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
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
	args.workerRes <- reason
	return
}

type testSagaCancel2 struct {
	gen.Saga
}

type testSagaCancel2Args struct {
	workerRes chan interface{}
	sagaRes   chan interface{}
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
	args.sagaRes <- id

	_, err := process.StartJob(id, gen.SagaJobOptions{}, process.State)
	if err != nil {
		panic(err)
	}
	args.workerRes <- id

	next := gen.SagaNext{}
	switch process.Name() {
	case "saga1":
		trapCancel, _ := value.(bool)
		if trapCancel {
			// case 2.D
			next.TrapCancel = true
		}
		next.Saga = gen.ProcessID{Name: "saga2", Node: "node2GenSagaCancelCases@localhost"}
	case "saga2":
		next.Saga = gen.ProcessID{Name: "saga3", Node: "node3GenSagaCancelCases@localhost"}
	default:
		return gen.SagaStatusOK
	}
	process.Next(id, next)

	return gen.SagaStatusOK
}

func (gs *testSagaCancel2) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	args := process.State.(testSagaCancel2Args)
	args.sagaRes <- reason
	return gen.SagaStatusOK
}

func (gs *testSagaCancel2) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
	return gen.SagaStatusOK
}

type testSagaStartTX struct {
	TrapCancel bool
}
type testSagaCancelTX struct {
	ID     gen.SagaTransactionID
	Reason string
}

func (gs *testSagaCancel2) HandleSagaInfo(process *gen.SagaProcess, message etf.Term) gen.ServerStatus {
	args := process.State.(testSagaCancel2Args)
	switch m := message.(type) {
	case gen.MessageSagaCancel:
		args.sagaRes <- m.Reason
		next := gen.SagaNext{}
		next.Saga = gen.ProcessID{Name: "saga4", Node: "node4GenSagaCancelCases@localhost"}
		process.Next(m.TransactionID, next)
		args.sagaRes <- m.TransactionID
	}
	return gen.ServerStatusOK
}

func (gs *testSagaCancel2) HandleSagaDirect(process *gen.SagaProcess, message interface{}) (interface{}, error) {

	switch m := message.(type) {
	case testSagaStartTX:
		return process.StartTransaction(gen.SagaTransactionOptions{}, m.TrapCancel), nil
	case testSagaCancelTX:
		return nil, process.CancelTransaction(m.ID, m.Reason)
	}
	return nil, nil
}

func TestSagaCancelCases(t *testing.T) {
	fmt.Printf("\n=== Test GenSagaCancelCases\n")

	fmt.Printf("Starting node: node1GenSagaCancelCases@localhost...")
	node1, _ := ergo.StartNode("node1GenSagaCancelCases@localhost", "cookies", node.Options{})

	if node1 == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
	defer node1.Stop()

	fmt.Printf("Starting node: node2GenSagaCancelCases@localhost...")
	node2, _ := ergo.StartNode("node2GenSagaCancelCases@localhost", "cookies", node.Options{})

	if node2 == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
	defer node2.Stop()

	fmt.Printf("Starting node: node3GenSagaCancelCases@localhost...")
	node3, _ := ergo.StartNode("node3GenSagaCancelCases@localhost", "cookies", node.Options{})

	if node3 == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
	defer node3.Stop()

	args1 := testSagaCancel2Args{
		workerRes: make(chan interface{}, 2),
		sagaRes:   make(chan interface{}, 2),
	}

	fmt.Printf("Starting saga1 on node1GenSagaCancelCases@localhost...")
	saga1 := &testSagaCancel2{}
	saga1_process, err := node1.Spawn("saga1", gen.ProcessOptions{MailboxSize: 10000}, saga1, args1)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	args2 := testSagaCancel2Args{
		workerRes: make(chan interface{}, 2),
		sagaRes:   make(chan interface{}, 2),
	}
	fmt.Printf("Starting saga2 on node2GenSagaCancelCases@localhost...")
	saga2 := &testSagaCancel2{}
	saga2_process, err := node2.Spawn("saga2", gen.ProcessOptions{MailboxSize: 10000}, saga2, args2)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	args3 := testSagaCancel2Args{
		workerRes: make(chan interface{}, 2),
		sagaRes:   make(chan interface{}, 2),
	}
	fmt.Printf("Starting saga3 on node3GenSagaCancelCases@localhost...")
	saga3 := &testSagaCancel2{}
	saga3_process, err := node3.Spawn("saga3", gen.ProcessOptions{MailboxSize: 10000}, saga3, args3)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	//
	// case 2.A
	//
	fmt.Println("  Case A (cancel TX on Node1.Saga1): Node1.Saga1 -> cancel -> Node2.Saga2 -> cancel -> Node3.Saga3")

	ValueTXID, err := saga1_process.Direct(testSagaStartTX{})
	if err != nil {
		t.Fatal(err)
	}
	TXID, ok := ValueTXID.(gen.SagaTransactionID)
	if !ok {
		t.Fatal("not a gen.SagaTransactionID")
	}

	fmt.Printf("... Start new TX %v on saga1: ", TXID)
	waitForResultWithValue(t, args1.sagaRes, TXID)
	fmt.Printf("... Start new worker on saga1 with TX %v: ", TXID)
	waitForResultWithValue(t, args1.workerRes, TXID)

	fmt.Printf("... Start new TX %v on saga2: ", TXID)
	waitForResultWithValue(t, args2.sagaRes, TXID)
	fmt.Printf("... Start new worker on saga2 with TX %v: ", TXID)
	waitForResultWithValue(t, args2.workerRes, TXID)

	fmt.Printf("... Start new TX %v on saga3: ", TXID)
	waitForResultWithValue(t, args3.sagaRes, TXID)
	fmt.Printf("... Start new worker on saga3 with TX %v: ", TXID)
	waitForResultWithValue(t, args3.workerRes, TXID)

	fmt.Printf("... Cancel TX %v on saga1: ", TXID)
	cancelReason := "cancel case1"
	_, err = saga1_process.Direct(testSagaCancelTX{ID: TXID, Reason: cancelReason})
	if err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, args1.sagaRes, cancelReason)
	fmt.Printf("...       saga1 cancels TX %v on its worker: ", TXID)
	waitForResultWithValue(t, args1.workerRes, cancelReason)
	fmt.Printf("...       cancels TX %v on saga2: ", TXID)
	waitForResultWithValue(t, args2.sagaRes, cancelReason)
	fmt.Printf("...       saga2 cancels TX %v on its worker: ", TXID)
	waitForResultWithValue(t, args2.workerRes, cancelReason)
	fmt.Printf("...       cancels TX %v on saga3: ", TXID)
	waitForResultWithValue(t, args3.sagaRes, cancelReason)
	fmt.Printf("...       saga3 cancels TX %v on its worker: ", TXID)
	waitForResultWithValue(t, args3.workerRes, cancelReason)
	//
	// case 2.B
	//

	fmt.Println("  Case B (cancel TX on Node.Saga2): Node1.Saga1 <- cancel <- Node2.Saga2 -> cancel -> Node3.Saga3")

	ValueTXID, err = saga1_process.Direct(testSagaStartTX{})
	if err != nil {
		t.Fatal(err)
	}
	TXID, ok = ValueTXID.(gen.SagaTransactionID)
	if !ok {
		t.Fatal("not a gen.SagaTransactionID")
	}

	fmt.Printf("... Start new TX %v on saga1: ", TXID)
	waitForResultWithValue(t, args1.sagaRes, TXID)
	fmt.Printf("... Start new worker on saga1 with TX %v: ", TXID)
	waitForResultWithValue(t, args1.workerRes, TXID)

	fmt.Printf("... Start new TX %v on saga2: ", TXID)
	waitForResultWithValue(t, args2.sagaRes, TXID)
	fmt.Printf("... Start new worker on saga2 with TX %v: ", TXID)
	waitForResultWithValue(t, args2.workerRes, TXID)

	fmt.Printf("... Start new TX %v on saga3: ", TXID)
	waitForResultWithValue(t, args3.sagaRes, TXID)
	fmt.Printf("... Start new worker on saga3 with TX %v: ", TXID)
	waitForResultWithValue(t, args3.workerRes, TXID)

	fmt.Printf("... Cancel TX %v on saga2: ", TXID)
	cancelReason = "cancel case2"
	_, err = saga2_process.Direct(testSagaCancelTX{ID: TXID, Reason: cancelReason})
	if err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, args2.sagaRes, cancelReason)
	fmt.Printf("...       saga2 cancels TX %v on its worker: ", TXID)
	waitForResultWithValue(t, args2.workerRes, cancelReason)
	fmt.Printf("...       cancels TX %v on saga1: ", TXID)
	waitForResultWithValue(t, args1.sagaRes, cancelReason)
	fmt.Printf("...       saga1 cancels TX %v on its worker: ", TXID)
	waitForResultWithValue(t, args1.workerRes, cancelReason)
	fmt.Printf("...       cancels TX %v on saga3: ", TXID)
	waitForResultWithValue(t, args3.sagaRes, cancelReason)
	fmt.Printf("...       saga3 cancels TX %v on its worker: ", TXID)
	waitForResultWithValue(t, args3.workerRes, cancelReason)
	//
	// case 2.C
	//
	fmt.Println("  Case C (cancel TX on Node.Saga3): Node1.Saga1 <- cancel <- Node2.Saga2 <- cancel <- Node3.Saga3")

	ValueTXID, err = saga1_process.Direct(testSagaStartTX{})
	if err != nil {
		t.Fatal(err)
	}
	TXID, ok = ValueTXID.(gen.SagaTransactionID)
	if !ok {
		t.Fatal("not a gen.SagaTransactionID")
	}

	fmt.Printf("... Start new TX %v on saga1: ", TXID)
	waitForResultWithValue(t, args1.sagaRes, TXID)
	fmt.Printf("... Start new worker on saga1 with TX %v: ", TXID)
	waitForResultWithValue(t, args1.workerRes, TXID)

	fmt.Printf("... Start new TX %v on saga2: ", TXID)
	waitForResultWithValue(t, args2.sagaRes, TXID)
	fmt.Printf("... Start new worker on saga2 with TX %v: ", TXID)
	waitForResultWithValue(t, args2.workerRes, TXID)

	fmt.Printf("... Start new TX %v on saga3: ", TXID)
	waitForResultWithValue(t, args3.sagaRes, TXID)
	fmt.Printf("... Start new worker on saga3 with TX %v: ", TXID)
	waitForResultWithValue(t, args3.workerRes, TXID)

	fmt.Printf("... Cancel TX %v on saga3: ", TXID)
	cancelReason = "cancel case3"
	_, err = saga3_process.Direct(testSagaCancelTX{ID: TXID, Reason: cancelReason})
	if err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, args3.sagaRes, cancelReason)
	fmt.Printf("...       saga3 cancels TX %v on its worker: ", TXID)
	waitForResultWithValue(t, args3.workerRes, cancelReason)
	fmt.Printf("...       cancels TX %v on saga2: ", TXID)
	waitForResultWithValue(t, args2.sagaRes, cancelReason)
	fmt.Printf("...       saga2 cancels TX %v on its worker: ", TXID)
	waitForResultWithValue(t, args2.workerRes, cancelReason)
	fmt.Printf("...       cancels TX %v on saga1: ", TXID)
	waitForResultWithValue(t, args1.sagaRes, cancelReason)
	fmt.Printf("...       saga1 cancels TX %v on its worker: ", TXID)
	waitForResultWithValue(t, args1.workerRes, cancelReason)
	//
	// Case 2.D
	//
	fmt.Println("  Case D: Saga1 sets TrapCancel, Saga2 process/node is going down, Saga1 sends Tx to the Saga4:")

	fmt.Printf("Starting node: node4GenSagaCancelCases@localhost...")
	node4, _ := ergo.StartNode("node4GenSagaCancelCases@localhost", "cookies", node.Options{})

	if node4 == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
	defer node4.Stop()

	args4 := testSagaCancel2Args{
		workerRes: make(chan interface{}, 2),
		sagaRes:   make(chan interface{}, 2),
	}
	fmt.Printf("Starting saga4 on node4GenSagaCancelCases@localhost...")
	saga4 := &testSagaCancel2{}
	saga4_process, err := node4.Spawn("saga4", gen.ProcessOptions{MailboxSize: 10000}, saga4, args4)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK", saga4_process.Self())

	// TrapCancel will be enabled on the Saga1 only
	ValueTXID, err = saga1_process.Direct(testSagaStartTX{TrapCancel: true})
	if err != nil {
		t.Fatal(err)
	}
	TXID, ok = ValueTXID.(gen.SagaTransactionID)
	if !ok {
		t.Fatal("not a gen.SagaTransactionID")
	}

	fmt.Printf("... Start new TX %v on saga1: ", TXID)
	waitForResultWithValue(t, args1.sagaRes, TXID)
	fmt.Printf("... Start new worker on saga1 with TX %v: ", TXID)
	waitForResultWithValue(t, args1.workerRes, TXID)

	fmt.Printf("... Start new TX %v on saga2: ", TXID)
	waitForResultWithValue(t, args2.sagaRes, TXID)
	fmt.Printf("... Start new worker on saga2 with TX %v: ", TXID)
	waitForResultWithValue(t, args2.workerRes, TXID)

	fmt.Printf("... Start new TX %v on saga3: ", TXID)
	waitForResultWithValue(t, args3.sagaRes, TXID)
	fmt.Printf("... Start new worker on saga3 with TX %v: ", TXID)
	waitForResultWithValue(t, args3.workerRes, TXID)

	fmt.Printf("... Terminate saga2 process: ")
	time.Sleep(200 * time.Millisecond)
	saga2_process.Kill()
	if err := saga2_process.WaitWithTimeout(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("...       handle trapped cancelation TX %v on saga1: ", TXID)
	cancelReason = fmt.Sprintf("next saga %s is down", gen.ProcessID{Name: saga2_process.Name(), Node: node2.Name()})
	waitForResultWithValue(t, args1.sagaRes, cancelReason)
	fmt.Printf("...       cancels TX %v on saga3: ", TXID)
	cancelReason = fmt.Sprintf("parent saga %s is down", saga2_process.Self())
	waitForResultWithValue(t, args3.sagaRes, cancelReason)
	fmt.Printf("...       saga3 cancels TX %v on its worker: ", TXID)
	waitForResultWithValue(t, args3.workerRes, cancelReason)

	fmt.Printf("...       forward (trapped) canceled TX %v on saga1 to Saga4: ", TXID)
	waitForResultWithValue(t, args1.sagaRes, TXID)
	fmt.Printf("... Start new TX %v on saga4: ", TXID)
	waitForResultWithValue(t, args4.sagaRes, TXID)
	fmt.Printf("... Start new worker on saga4 with TX %v: ", TXID)
	waitForResultWithValue(t, args4.workerRes, TXID)
}
