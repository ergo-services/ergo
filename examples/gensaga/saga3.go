package main

import (
	"fmt"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

//
// Saga3 worker
//
type saga3worker struct {
	gen.SagaWorker
}

func (w *saga3worker) HandleJobStart(process *gen.SagaWorkerProcess, job gen.SagaJob) error {
	fmt.Printf(" Worker started on Saga3 with value %q\n", job.Value)
	s := job.Value.(string) + "d"
	process.SendResult(s)
	process.State = job.TransactionID
	return nil
}
func (w *saga3worker) HandleJobCancel(process *gen.SagaWorkerProcess, reason string) {
	return
}
func (w *saga3worker) HandleJobCommit(process *gen.SagaWorkerProcess, final interface{}) {
	txid := process.State.(gen.SagaTransactionID)
	fmt.Printf(" Worker on Saga3 received final result for %v with value %q\n", txid, final)
}

func (w *saga3worker) HandleWorkerTerminate(process *gen.SagaWorkerProcess, reason string) {
	fmt.Printf(" Worker terminated on Saga3 with reason: %q\n", reason)
}

//
// Saga3
//
type Saga3 struct {
	gen.Saga
}

func (s *Saga3) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{
		Worker: &saga3worker{},
	}

	return opts, nil
}

func (s *Saga3) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	// add some sleep
	fmt.Printf("Saga3. Received %v with value %q\n", id, value)
	time.Sleep(300 * time.Millisecond)
	process.StartJob(id, gen.SagaJobOptions{}, value)
	return gen.SagaStatusOK
}

func (s *Saga3) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	fmt.Printf("Saga3. Canceled %v with reason: %q\n", id, reason)
	return gen.SagaStatusOK
}

func (s *Saga3) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
	return gen.SagaStatusOK
}

func (s *Saga3) HandleJobResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaJobID, result interface{}) gen.SagaStatus {

	fmt.Printf("Saga3. Received result from worker with value %q\n", result)
	r := result.(string) + "!"
	if err := process.SendResult(id, r); err != nil {
		fmt.Printf("Saga3. ...can't send result %q to the parent saga for %v due to: %s\n", r, id, err)

	} else {
		fmt.Printf("Saga3. ...sent result %q to the parent saga for %v\n", r, id)
	}
	return gen.SagaStatusOK
}

func (s *Saga3) HandleTxCommit(process *gen.SagaProcess, id gen.SagaTransactionID, final interface{}) gen.SagaStatus {
	fmt.Printf("Saga3. Final result for %v: %q\n", id, final)
	return gen.SagaStatusOK
}
