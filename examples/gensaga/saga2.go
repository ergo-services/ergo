package main

import (
	"fmt"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

//
// Saga2 worker
//
type saga2worker struct {
	gen.SagaWorker
}

func (w *saga2worker) HandleJobStart(process *gen.SagaWorkerProcess, job gen.SagaJob) error {
	fmt.Printf(" Worker started on Saga2 with value %q\n", job.Value)
	s := job.Value.(string) + "r"
	process.SendResult(s)
	process.State = job.TransactionID
	return nil
}
func (w *saga2worker) HandleJobCancel(process *gen.SagaWorkerProcess, reason string) {
	return
}
func (w *saga2worker) HandleJobCommit(process *gen.SagaWorkerProcess, final interface{}) {
	txid := process.State.(gen.SagaTransactionID)
	fmt.Printf(" Worker on Saga2 received final result for %v with value %q\n", txid, final)
}

func (w *saga2worker) HandleWorkerTerminate(process *gen.SagaWorkerProcess, reason string) {
	fmt.Printf(" Worker terminated on Saga2 with reason: %q\n", reason)
}

//
// Saga2
//
type Saga2 struct {
	gen.Saga
}

func (s *Saga2) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{
		Worker: &saga2worker{},
	}

	return opts, nil
}

func (s *Saga2) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	// add some sleep
	fmt.Printf("Saga2. Received %v with value %q\n", id, value)
	time.Sleep(300 * time.Millisecond)
	process.StartJob(id, gen.SagaJobOptions{}, value)
	return gen.SagaStatusOK
}

func (s *Saga2) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (s *Saga2) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (s *Saga2) HandleJobResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaJobID, result interface{}) gen.SagaStatus {

	fmt.Printf("Saga2. Received result from worker with value %q\n", result)
	next := gen.SagaNext{
		Saga:  gen.ProcessID{Name: "saga3", Node: "node3@localhost"},
		Value: result.(string) + "l",
	}
	next_id, _ := process.Next(id, next)
	fmt.Printf("Saga2. ...sent %v further, to the Saga3 on Node3 (%v)\n", id, next_id)

	fmt.Println("Saga2 termination")
	return gen.SagaStatusStop
}
