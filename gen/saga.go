package gen

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
)

// SagaBehavior interface
type SagaBehavior interface {
	//
	// Mandatory callbacks
	//

	// InitSaga
	InitSaga(process *SagaProcess, args ...etf.Term) (SagaOptions, error)

	// HandleTxNew invokes on a new TX receiving by this saga.
	HandleTxNew(process *SagaProcess, tx SagaTransaction, value interface{}) SagaStatus

	// HandleTxCancel invoked on a request of transaction cancelation.
	HandleTxCancel(process *SagaProcess, tx SagaTransaction, reason string) SagaStatus

	// HandleTxResult invoked on a receiving result from the next saga
	HandleTxResult(process *SagaProcess, tx SagaTransaction, next SagaNext, result interface{}) SagaStatus

	// HandleTxTimeout invoked if a result haven't been recieved from the next saga in time.
	HandleTxTimeout(process *SagaProcess, tx SagaTransaction, next SagaNext) SagaStatus

	//
	// Optional callbacks
	//

	// HandleInterim invoked if received interim result from the Next hop
	HandleTxInterim(process *SagaProcess, tx SagaTransaction, next SagaNext, interim interface{}) SagaStatus

	// HandleDone invoked when the TX is done. Invoked on a saga where this tx was created.
	HandleTxDone(process *SagaProcess, tx SagaTransaction)

	//
	// Callbacks to handle results from the worker(s)
	//

	// HandleJobResult
	HandleJobResult(process *SagaProcess, id SagaJobID, result interface{}) SagaStatus
	// HandleJobInterim
	HandleJobInterim(process *SagaProcess, id SagaJobID, interim interface{}) SagaStatus
	// HandleJobFailed
	HandleJobFailed(process *SagaProcess, id SagaJobID) SagaStatus

	//
	// Server's callbacks
	//

	// HandleStageCall this callback is invoked on Process.Call. This method is optional
	// for the implementation
	HandleSagaCall(process *SagaProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus)
	// HandleStageCast this callback is invoked on Process.Cast. This method is optional
	// for the implementation
	HandleSagaCast(process *SagaProcess, message etf.Term) ServerStatus
	// HandleStageInfo this callback is invoked on Process.Send. This method is optional
	// for the implementation
	HandleSagaInfo(process *SagaProcess, message etf.Term) ServerStatus
}

type SagaStatus error

var (
	SagaStatusOK          SagaStatus // nil
	SagaStatusStop        SagaStatus = fmt.Errorf("stop")
	sagaStatusUnsupported SagaStatus = fmt.Errorf("unsupported")
)

type Saga struct {
	Server
}

type SagaTransactionOptions struct {
	// HopLimit defines a number of hop within the transaction. Default limit
	// is 0 (no limit).
	HopLimit uint
	// Lifespan defines a lifespan for the transaction in seconds. Default 0 (no limit)
	Lifespan uint

	// TwoPhaseCommit enables 2PC for the transaction. This option makes all
	// Sagas involved in this transaction invoke HandleCommit on them and
	// invoke HandleCommitJob callback on Worker processes once the transaction is finished.
	TwoPhaseCommit bool
}

type SagaOptions struct {
	// MaxTransactions defines the limit for the number of active transactions. Default: 0 (unlimited)
	MaxTransactions uint
	// Worker
	Worker SagaWorkerBehavior
}

type SagaProcess struct {
	ServerProcess
	options SagaOptions

	// running transactions
	txs      map[etf.Ref]SagaTransaction
	mutexTXS sync.Mutex

	// next sagas where txs were sent
	next      map[etf.Ref]SagaNext
	mutexNext sync.Mutex

	// running jobs
	jobs      map[SagaJobID]Process
	mutexJobs sync.Mutex
}

type SagaTransaction struct {
	ID        etf.Ref
	Name      string
	StartTime int64
	Options   SagaTransactionOptions
	Parents   []etf.Pid

	// internal
	context context.Context
	cancel  context.CancelFunc
}

type SagaNext struct {
	// Saga - etf.Pid, string (for the locally registered process), gen.ProcessID{process, node} (for the remote process)
	Saga interface{}
	// Value - a value for the invoking HandleTX on a Next hop.
	Value interface{}
	// Timeout - how long this Saga will be waiting for the result from the Next hop. Default - 10 seconds
	Timeout uint

	id etf.Ref
}

type SagaJobID etf.Ref

type SagaJob struct {
	ID      SagaJobID
	Value   interface{}
	options SagaJobOptions
	commit  bool
	saga    etf.Pid
}

type SagaJobOptions struct {
	Timeout uint
}

type messageSaga struct {
	request string
	pid     etf.Pid
	command interface{}
}

type messageSagaNext struct {
	Transaction SagaTransaction
	Ref         etf.Ref
	Value       interface{}
}

type messageSagaResult struct {
	Transaction SagaTransaction
	From        etf.Pid
	Result      interface{}
}

type messageSagaCancel struct {
	ID     etf.Ref
	Name   string
	Reason string
}

//
// Saga API
//

type sagaSetMaxTransactions struct {
	max uint
}

// SetMaxTransactions set maximum transactions fo the saga
func (gs *Saga) SetMaxTransactions(process Process, max uint) error {
	if !process.IsAlive() {
		return ErrServerTerminated
	}
	message := sagaSetMaxTransactions{
		max: max,
	}
	_, err := process.Direct(message)
	return err
}

//
// SagaProcess methods
//

func (sp *SagaProcess) StartTransaction(name string, options SagaTransactionOptions, value interface{}) (etf.Ref, error) {
	if len(sp.txs)+1 > int(sp.options.MaxTransactions) {
		return etf.Ref{}, fmt.Errorf("exceed_tx_limit")
	}

	if name == "" {
		// must be enought for the unique name
		name = lib.RandomString(32)
	}

	// use reference as a transaction ID
	id := sp.MakeRef()
	tx := SagaTransaction{
		ID:        id,
		Name:      name,
		Options:   options,
		StartTime: time.Now().Unix(),
	}

	sp.mutexTXS.Lock()
	sp.txs[id] = tx
	sp.mutexTXS.Unlock()

	message := etf.Tuple{
		etf.Atom("$saga_next"),
		sp.Self(),
		etf.Tuple{tx, etf.Ref{}, value},
	}
	sp.Send(sp.Self(), message)

	tx.context, tx.cancel = context.WithCancel(sp.Context())

	if options.Lifespan > 0 {
		ctx, _ := context.WithTimeout(tx.context, time.Duration(options.Lifespan)*time.Second)
		go func() {
			<-ctx.Done()
			sp.CancelTransaction(id, "timeout")
		}()
	}

	return id, nil

}

func (sp *SagaProcess) CancelTransaction(id etf.Ref, reason string) {
	sp.mutexTXS.Lock()
	_, ok := sp.txs[id]
	if !ok {
		sp.mutexTXS.Unlock()
		return
	}
	delete(sp.txs, id)
	sp.mutexTXS.Unlock()

	message := etf.Tuple{
		etf.Atom("$saga_cancel"),
		sp.Self(),
		etf.Tuple{id, etf.Ref{}, reason},
	}
	sp.Send(sp.Self(), message)

}

func (sp *SagaProcess) CommitTransaction(tx SagaTransaction) {

}

func (sp *SagaProcess) Next(tx SagaTransaction, next SagaNext) {

}

func (sp *SagaProcess) StartJob(tx SagaTransaction, options SagaJobOptions, value interface{}) (SagaJobID, error) {
	job := SagaJob{}

	if sp.options.Worker == nil {
		return job.ID, fmt.Errorf("This saga has no worker")
	}
	// make context WithTimeout to limit the lifespan
	workerOptions := ProcessOptions{}
	worker, err := sp.Spawn("", workerOptions, sp.options.Worker)
	if err != nil {
		return job.ID, err
	}
	ref := sp.MonitorProcess(worker.Self())
	job.ID = SagaJobID(ref)
	job.Value = value
	job.commit = tx.Options.TwoPhaseCommit

	return job.ID, nil
}

func (sp *SagaProcess) CancelJob(job SagaJobID) error {
	return nil

}

func (sp *SagaProcess) SendResult(tx SagaTransaction, result interface{}) {

}

func (sp *SagaProcess) SendInterim(tx SagaTransaction, interim interface{}) {

}

//
// Server callbacks
//
func (gs *Saga) Init(process *ServerProcess, args ...etf.Term) error {
	var options SagaOptions
	behavior := process.Behavior().(SagaBehavior)
	//behavior, ok := process.Behavior().(SagaBehavior)
	//if !ok {
	//	return fmt.Errorf("Saga: not a SagaBehavior")
	//}

	sagaProcess := &SagaProcess{
		ServerProcess: *process,
		txs:           make(map[etf.Ref]SagaTransaction),
	}
	// do not inherite parent State
	sagaProcess.State = nil

	options, err := behavior.InitSaga(sagaProcess, args...)
	if err != nil {
		return err
	}

	process.State = sagaProcess

	if options.Worker != nil {
		sagaProcess.jobs = make(map[SagaJobID]Process)
	}

	return nil
}

func (gs *Saga) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	sp := process.State.(*SagaProcess)
	return process.Behavior().(SagaBehavior).HandleSagaCall(sp, from, message)
}

func (gs *Saga) HandleDirect(process *ServerProcess, message interface{}) (interface{}, ServerStatus) {
	st := process.State.(*SagaProcess)
	switch m := message.(type) {
	case sagaSetMaxTransactions:
		st.options.MaxTransactions = m.max
		return nil, nil
	default:
		return nil, ErrUnsupportedRequest
	}
}

func (gs *Saga) HandleCast(process *ServerProcess, message etf.Term) ServerStatus {
	var status SagaStatus
	st := process.State.(*SagaProcess)
	switch m := message.(type) {
	case messageSagaJobResult:
		status = process.Behavior().(SagaBehavior).HandleJobResult(st, m.id, m.result)
	case messageSagaJobInterim:
		status = process.Behavior().(SagaBehavior).HandleJobInterim(st, m.id, m.interim)
	default:
		s := process.Behavior().(SagaBehavior).HandleSagaCast(st, message)
		status = SagaStatus(s)
	}
	switch status {
	case SagaStatusOK:
		return ServerStatusOK
	case SagaStatusStop:
		return ServerStatusStop
	default:
		return ServerStatus(status)
	}
}

func (gs *Saga) HandleInfo(process *ServerProcess, message etf.Term) ServerStatus {
	var m messageSaga

	st := process.State.(*SagaProcess)
	// check if we got a MessageDown
	if d, isDown := IsMessageDown(message); isDown {
		if err := handleSagaDown(st, d); err != nil {
			return ServerStatus(err)
		}
		return ServerStatusOK
	}

	if err := etf.TermIntoStruct(message, &m); err != nil {
		status := process.Behavior().(SagaBehavior).HandleSagaInfo(st, message)
		return status
	}

	status := handleSagaRequest(st, m)
	switch status {
	case nil:
		return ServerStatusOK
	case SagaStatusStop:
		return ServerStatusStop
	case sagaStatusUnsupported:
		return process.Behavior().(SagaBehavior).HandleSagaInfo(st, message)
	default:
		return ServerStatus(status)
	}
}

func handleSagaRequest(process *SagaProcess, m messageSaga) error {
	var nextMessage messageSagaNext
	var cancel messageSagaCancel
	var result messageSagaResult

	next := SagaNext{}
	switch m.request {
	case "$saga_next":
		if err := etf.TermIntoStruct(m.command, &nextMessage); err != nil {
			return ErrUnsupportedRequest
		}

		// Check for the loop
		if _, ok := process.txs[nextMessage.Transaction.ID]; ok {
			cancel := etf.Tuple{
				etf.Atom("$saga_cancel"),
				process.Self(),
				etf.Tuple{
					nextMessage.Transaction.ID,
					nextMessage.Ref,
					"loop_detected",
				},
			}
			process.Send(m.pid, cancel)
			return nil
		}

		// Check if exceed the number of transaction on this saga
		if len(process.txs)+1 > int(process.options.MaxTransactions) {
			cancel := etf.Tuple{
				etf.Atom("$saga_cancel"),
				process.Self(),
				etf.Tuple{
					nextMessage.Transaction.ID,
					nextMessage.Ref,
					"exceed_tx_limit",
				},
			}
			process.Send(m.pid, cancel)
			return nil
		}

		// Check if exceed the hop limit
		hop := len(nextMessage.Transaction.Parents)
		hoplimit := nextMessage.Transaction.Options.HopLimit
		if hoplimit > 0 && hop+1 > int(hoplimit) {
			cancel := etf.Tuple{
				etf.Atom("$saga_cancel"),
				process.Self(),
				etf.Tuple{
					nextMessage.Transaction.ID,
					nextMessage.Ref,
					"exceed_hop_limit",
				},
			}
			process.Send(m.pid, cancel)
			return nil
		}

		// Check if lifespan is limited and transaction is too long
		lifespan := nextMessage.Transaction.Options.Lifespan
		l := time.Now().Unix() - nextMessage.Transaction.StartTime
		if lifespan > 0 && l > int64(lifespan) {
			cancel := etf.Tuple{
				etf.Atom("$saga_cancel"),
				process.Self(),
				etf.Tuple{
					nextMessage.Transaction.ID,
					nextMessage.Ref,
					"exceed_lifespan",
				},
			}
			process.Send(m.pid, cancel)
			return nil
		}

		// everything looks good. go further
		process.txs[nextMessage.Transaction.ID] = nextMessage.Transaction

		return process.Behavior().(SagaBehavior).HandleTxNew(process, nextMessage.Transaction, next.Value)

	case "$saga_cancel":
		if err := etf.TermIntoStruct(m.command, &cancel); err != nil {
			return ErrUnsupportedRequest
		}
		tx, exist := process.txs[cancel.ID]
		if !exist {
			return nil
		}

		process.Behavior().(SagaBehavior).HandleTxCancel(process, tx, cancel.Reason)
		return nil
	case "$saga_interim":
		if err := etf.TermIntoStruct(m.command, &result); err != nil {
			return ErrUnsupportedRequest
		}
		process.Behavior().(SagaBehavior).HandleTxInterim(process, result.Transaction, next, result.Result)
		return nil
	case "$saga_result":
		if err := etf.TermIntoStruct(m.command, &result); err != nil {
			return ErrUnsupportedRequest
		}
		process.Behavior().(SagaBehavior).HandleTxResult(process, result.Transaction, next, result.Result)
		return nil
	}
	return sagaStatusUnsupported
}

func handleSagaDown(process *SagaProcess, down MessageDown) error {
	return nil
}

//
// default Saga callbacks
//
func (gs *Saga) HandleCommit(process *SagaProcess, tx SagaTransaction) {
	return
}
func (gs *Saga) HandleInterim(process *SagaProcess, tx SagaTransaction, interim interface{}) error {
	// default callback if it wasn't implemented
	fmt.Printf("HandleInterim: unhandled message %#v\n", tx)
	return nil
}
func (gs *Saga) HandleSagaCall(process *SagaProcess, from ServerFrom, message etf.Term) (string, etf.Term) {
	// default callback if it wasn't implemented
	fmt.Printf("HandleSagaCall: unhandled message (from %#v) %#v\n", from, message)
	return "reply", etf.Atom("ok")
}
func (gs *Saga) HandleSagaCast(process *SagaProcess, message etf.Term) string {
	// default callback if it wasn't implemented
	fmt.Printf("HandleSagaCast: unhandled message %#v\n", message)
	return "noreply"
}
func (gs *Saga) HandleSagaInfo(process *SagaProcess, message etf.Term) string {
	// default callback if it wasn't implemnted
	fmt.Printf("HandleSagaInfo: unhandled message %#v\n", message)
	return "noreply"
}
func (gs *Saga) HandleJobResult(process *SagaProcess, id SagaJobID, result interface{}) error {
	fmt.Printf("HandleJobResult: unhandled message %#v\n", result)
	return nil
}
func (gs *Saga) HandleJobInterim(process *SagaProcess, id SagaJobID, interim interface{}) error {
	fmt.Printf("HandleJobInterim: unhandled message %#v\n", interim)
	return nil
}
func (gs *Saga) HandleJobFailed(process *SagaProcess, id SagaJobID) error {
	fmt.Printf("HandleJobFailed: unhandled message %#v\n", id)
	return nil
}
