package tests

import (
	"fmt"
	"testing"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

type argsSagaCommitArgs struct {
	workerRes    chan interface{}
	sagaRes      chan interface{}
	testCaseDist bool
}

type testSagaCommitWorker1 struct {
	gen.SagaWorker
}

func (w *testSagaCommitWorker1) HandleJobStart(process *gen.SagaWorkerProcess, job gen.SagaJob) error {
	process.State = job.Value
	process.SendResult(123)
	args := process.State.(argsSagaCommitArgs)
	args.workerRes <- "jobresult"
	return nil
}
func (w *testSagaCommitWorker1) HandleJobCommit(process *gen.SagaWorkerProcess, final interface{}) {
	args := process.State.(argsSagaCommitArgs)
	args.workerRes <- final
	return
}
func (w *testSagaCommitWorker1) HandleJobCancel(process *gen.SagaWorkerProcess, reason string) {
	return
}

func (w *testSagaCommitWorker1) HandleWorkerTerminate(process *gen.SagaWorkerProcess, reason string) {
	args := process.State.(argsSagaCommitArgs)
	args.workerRes <- reason
}

type testSagaCommit1 struct {
	gen.Saga
}

func (gs *testSagaCommit1) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	worker := &testSagaCommitWorker1{}
	opts := gen.SagaOptions{
		Worker: worker,
	}
	process.State = args[0]
	return opts, nil
}

func (gs *testSagaCommit1) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	args := process.State.(argsSagaCommitArgs)
	args.sagaRes <- "newtx"

	_, err := process.StartJob(id, gen.SagaJobOptions{}, args)
	if err != nil {
		panic(err)
	}

	if args.testCaseDist {
		next := gen.SagaNext{
			Saga: gen.ProcessID{Name: "saga2", Node: "nodeGenSagaCommitDist02@localhost"},
		}
		process.Next(id, next)
	}

	return gen.SagaStatusOK
}

func (gs *testSagaCommit1) HandleTxDone(process *gen.SagaProcess, id gen.SagaTransactionID, result interface{}) (interface{}, gen.SagaStatus) {
	args := process.State.(argsSagaCommitArgs)
	args.sagaRes <- "txdone"
	return 6.28, gen.SagaStatusOK
}

func (gs *testSagaCommit1) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSagaCommit1) HandleTxCommit(process *gen.SagaProcess, id gen.SagaTransactionID, final interface{}) gen.SagaStatus {
	args := process.State.(argsSagaCommitArgs)
	args.sagaRes <- final
	return gen.SagaStatusOK
}

func (gs *testSagaCommit1) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
	args := process.State.(argsSagaCommitArgs)
	args.sagaRes <- "txresult"
	return gen.SagaStatusOK
}

func (gs *testSagaCommit1) HandleJobResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaJobID, result interface{}) gen.SagaStatus {
	return gen.SagaStatusOK
}

type testSagaCommitStartTx struct{}
type testSagaCommitSendRes struct {
	id gen.SagaTransactionID
}

func (gs *testSagaCommit1) HandleSagaDirect(process *gen.SagaProcess, message interface{}) (interface{}, error) {

	switch m := message.(type) {
	case testSagaCommitStartTx:
		id := process.StartTransaction(gen.SagaTransactionOptions{TwoPhaseCommit: true}, m)
		return id, nil
	case testSagaCommitSendRes:
		return nil, process.SendResult(m.id, 3.14)
	}
	return nil, nil
}

func TestSagaCommitSimple(t *testing.T) {

	fmt.Printf("\n=== Test GenSagaCommitSimple\n")
	fmt.Printf("Starting node: nodeGenSagaCommitSimple01@localhost...")

	node, _ := ergo.StartNode("nodeGenSagaCommitSimple01@localhost", "cookies", node.Options{})
	if node == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
	defer node.Stop()

	fmt.Printf("... Starting Saga processes: ")
	args := argsSagaCommitArgs{
		workerRes: make(chan interface{}, 2),
		sagaRes:   make(chan interface{}, 2),
	}
	saga := &testSagaCommit1{}
	saga_process, err := node.Spawn("saga", gen.ProcessOptions{MailboxSize: 10000}, saga, args)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	ValueTXID, err := saga_process.Direct(testSagaCommitStartTx{})
	if err != nil {
		t.Fatal(err)
	}
	TXID, ok := ValueTXID.(gen.SagaTransactionID)
	if !ok {
		t.Fatal("not a gen.SagaTransactionID")
	}
	fmt.Printf("... Start new TX on saga: ")
	waitForResultWithValue(t, args.sagaRes, "newtx")
	fmt.Printf("... Start new worker on saga: ")
	waitForResultWithValue(t, args.workerRes, "jobresult")
	fmt.Printf("... Sending result on saga: ")
	if _, err := saga_process.Direct(testSagaCommitSendRes{id: TXID}); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	fmt.Printf("... Handle TX done on saga: ")
	waitForResultWithValue(t, args.sagaRes, "txdone")
	fmt.Printf("... Handle TX commit with final value on worker: ")
	waitForResultWithValue(t, args.workerRes, 6.28)
	fmt.Printf("... Worker terminated: ")
	waitForResultWithValue(t, args.workerRes, "normal")
}

func TestSagaCommitDistributed(t *testing.T) {

	fmt.Printf("\n=== Test GenSagaCommitDistributed\n")

	fmt.Printf("Starting node: nodeGenSagaCommitDist01@localhost...")
	node1, _ := ergo.StartNode("nodeGenSagaCommitDist01@localhost", "cookies", node.Options{})
	if node1 == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
	defer node1.Stop()

	fmt.Printf("Starting node: nodeGenSagaCommitDist02@localhost...")
	node2, _ := ergo.StartNode("nodeGenSagaCommitDist02@localhost", "cookies", node.Options{})
	if node2 == nil {
		t.Fatal("can't start node")
		return
	}
	fmt.Println("OK")
	defer node2.Stop()

	args1 := argsSagaCommitArgs{
		workerRes:    make(chan interface{}, 2),
		sagaRes:      make(chan interface{}, 2),
		testCaseDist: true,
	}
	fmt.Printf("... Starting Saga1 processes on node1: ")
	saga1 := &testSagaCommit1{}
	saga1_process, err := node1.Spawn("saga1", gen.ProcessOptions{MailboxSize: 10000}, saga1, args1)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	args2 := argsSagaCommitArgs{
		workerRes: make(chan interface{}, 2),
		sagaRes:   make(chan interface{}, 2),
	}
	fmt.Printf("... Starting Saga2 processes on node2: ")
	saga2 := &testSagaCommit1{}
	saga2_process, err := node2.Spawn("saga2", gen.ProcessOptions{MailboxSize: 10000}, saga2, args2)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	ValueTXID, err := saga1_process.Direct(testSagaCommitStartTx{})
	if err != nil {
		t.Fatal(err)
	}
	TXID, ok := ValueTXID.(gen.SagaTransactionID)
	if !ok {
		t.Fatal("not a gen.SagaTransactionID")
	}
	fmt.Printf("... Start new TX on saga1: ")
	waitForResultWithValue(t, args1.sagaRes, "newtx")
	fmt.Printf("... Start new worker on saga1: ")
	waitForResultWithValue(t, args1.workerRes, "jobresult")
	fmt.Printf("... Start new TX on saga2: ")
	waitForResultWithValue(t, args2.sagaRes, "newtx")
	fmt.Printf("... Start new worker on saga2: ")
	waitForResultWithValue(t, args2.workerRes, "jobresult")
	fmt.Printf("... Try to send the result on saga1 (must be error ErrSagaTxInProgress): ")
	if _, err := saga1_process.Direct(testSagaCommitSendRes{id: TXID}); err != gen.ErrSagaTxInProgress {
		t.Fatal("must be error here")
	}
	fmt.Println("OK")

	fmt.Printf("... Send the result on saga2 : ")
	if _, err := saga2_process.Direct(testSagaCommitSendRes{id: TXID}); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... Handle TX result on saga1 (from saga2): ")
	waitForResultWithValue(t, args1.sagaRes, "txresult")
	fmt.Printf("... Send the result on saga1 : ")
	if _, err := saga1_process.Direct(testSagaCommitSendRes{id: TXID}); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	fmt.Printf("... Handle TX done on saga1: ")
	waitForResultWithValue(t, args1.sagaRes, "txdone")
	fmt.Printf("... Handle TX commit with final value on saga1 worker: ")
	waitForResultWithValue(t, args1.workerRes, 6.28)
	fmt.Printf("... Handle TX commit on saga2: ")
	waitForResultWithValue(t, args2.sagaRes, 6.28)
	fmt.Printf("... Handle TX commit with final value on saga2 worker: ")
	waitForResultWithValue(t, args2.workerRes, 6.28)
}
