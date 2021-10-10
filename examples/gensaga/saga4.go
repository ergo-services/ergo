package main

import (
	"fmt"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

//
// Saga4 worker
//
type saga4worker struct {
	gen.SagaWorker
}

func (w *saga4worker) HandleJobStart(process *gen.SagaWorkerProcess, job gen.SagaJob) error {
	fmt.Printf(" Worker started on Saga4 with value %q\n", job.Value)
	s := job.Value.(string) + "r"
	process.SendResult(s)
	process.State = job.TransactionID
	return nil
}
func (w *saga4worker) HandleJobCancel(process *gen.SagaWorkerProcess, reason string) {
	return
}
func (w *saga4worker) HandleJobCommit(process *gen.SagaWorkerProcess, final interface{}) {
	txid := process.State.(gen.SagaTransactionID)
	fmt.Printf(" Worker on Saga4 received final result for %v with value %q\n", txid, final)
}

func (w *saga4worker) HandleWorkerTerminate(process *gen.SagaWorkerProcess, reason string) {
	fmt.Printf(" Worker terminated on Saga4 with reason: %q\n", reason)
}

//
// Saga4 (the same as Saga2, but with no termination)
//
type Saga4 struct {
	gen.Saga
}

func (s *Saga4) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{
		Worker: &saga4worker{},
	}

	return opts, nil
}

func (s *Saga4) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	// add some sleep
	fmt.Printf("Saga4. Received %v with value %q\n", id, value)
	time.Sleep(300 * time.Millisecond)
	process.StartJob(id, gen.SagaJobOptions{}, value)
	return gen.SagaStatusOK
}

func (s *Saga4) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (s *Saga4) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
	fmt.Printf("Saga4. Received result for %v from Saga3 (%v) with value %q\n", id, from, result)
	process.SendResult(id, result)
	fmt.Printf("Saga4. ...sent result %q to the parent saga for %v\n", result, id)
	return gen.SagaStatusOK
}

func (s *Saga4) HandleJobResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaJobID, result interface{}) gen.SagaStatus {

	fmt.Printf("Saga4. Received result from worker with value %q\n", result)
	next := gen.SagaNext{
		Saga:  gen.ProcessID{Name: "saga3", Node: "node3@localhost"},
		Value: result.(string) + "l",
	}
	next_id, _ := process.Next(id, next)
	fmt.Printf("Saga4. ...sent %v further, to the Saga3 on Node3 (%v)\n", id, next_id)
	return gen.SagaStatusOK
}

func (s *Saga4) HandleTxCommit(process *gen.SagaProcess, id gen.SagaTransactionID, final interface{}) gen.SagaStatus {
	fmt.Printf("Saga4. Final result for %v: %q\n", id, final)
	return gen.SagaStatusOK
}
