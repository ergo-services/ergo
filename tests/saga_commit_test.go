package tests

import (
	"fmt"
	"testing"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
)

type taskSagaCommitCase1 struct {
	workerRes chan interface{}
	sagaRes   chan interface{}
}

type testSagaCommitWorker1 struct {
	gen.SagaWorker
}

func (w *testSagaCommitWorker1) HandleJobStart(process *gen.SagaWorkerProcess, job gen.SagaJob) error {
	process.State = job.Value
	process.SendResult(123)
	task := process.State.(taskSagaCommitCase1)
	task.workerRes <- "jobresult"
	return nil
}
func (w *testSagaCommitWorker1) HandleJobCommit(process *gen.SagaWorkerProcess, final interface{}) {
	task := process.State.(taskSagaCommitCase1)
	task.workerRes <- final
	return
}
func (w *testSagaCommitWorker1) HandleJobCancel(process *gen.SagaWorkerProcess, reason string) {
	return
}

func (w *testSagaCommitWorker1) HandleWorkerTerminate(process *gen.SagaWorkerProcess, reason string) {
	task := process.State.(taskSagaCommitCase1)
	task.workerRes <- reason
}

type testSagaCommit1 struct {
	gen.Saga
}

func (gs *testSagaCommit1) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	worker := &testSagaCommitWorker1{}
	opts := gen.SagaOptions{
		Worker: worker,
	}
	return opts, nil
}

func (gs *testSagaCommit1) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	process.State = value
	task := process.State.(taskSagaCommitCase1)
	task.sagaRes <- "newtx"

	_, err := process.StartJob(id, gen.SagaJobOptions{}, value)
	if err != nil {
		panic(err)
	}

	return gen.SagaStatusOK
}

func (gs *testSagaCommit1) HandleTxDone(process *gen.SagaProcess, id gen.SagaTransactionID, result interface{}) (interface{}, gen.SagaStatus) {
	task := process.State.(taskSagaCommitCase1)
	task.sagaRes <- "txdone"
	return 6.28, gen.SagaStatusOK
}

func (gs *testSagaCommit1) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (gs *testSagaCommit1) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
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
	case taskSagaCommitCase1:
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
	saga := &testSagaCommit1{}
	saga_process, err := node.Spawn("saga", gen.ProcessOptions{MailboxSize: 10000}, saga)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	task := taskSagaCommitCase1{
		workerRes: make(chan interface{}, 2),
		sagaRes:   make(chan interface{}, 2),
	}
	ValueTXID, err := saga_process.Direct(task)
	if err != nil {
		t.Fatal(err)
	}
	TXID, ok := ValueTXID.(gen.SagaTransactionID)
	if !ok {
		t.Fatal("not a gen.SagaTransactionID")
	}
	fmt.Printf("... Start new TX on saga: ")
	waitForResultWithValue(t, task.sagaRes, "newtx")
	fmt.Printf("... Start new worker on saga: ")
	waitForResultWithValue(t, task.workerRes, "jobresult")
	fmt.Printf("... Sending result on saga: ")
	if _, err := saga_process.Direct(testSagaCommitSendRes{id: TXID}); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	fmt.Printf("... Handle TX done on saga: ")
	waitForResultWithValue(t, task.sagaRes, "txdone")
	fmt.Printf("... Handle TX commit with final value on worker: ")
	waitForResultWithValue(t, task.workerRes, 6.28)
	fmt.Printf("... Worker terminated: ")
	waitForResultWithValue(t, task.workerRes, "normal")
}
