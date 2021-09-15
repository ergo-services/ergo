package gen

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/halturin/ergo/etf"
)

// SagaBehavior interface
type SagaBehavior interface {
	//
	// Mandatory callbacks
	//

	// InitSaga
	InitSaga(process *SagaProcess, args ...etf.Term) (SagaOptions, error)

	// HandleTxNew invokes on a new TX receiving by this saga.
	HandleTxNew(process *SagaProcess, id SagaTransactionID, value interface{}) SagaStatus

	// HandleTxResult invoked on a receiving result from the next saga
	HandleTxResult(process *SagaProcess, id SagaTransactionID, from SagaNextID, result interface{}) SagaStatus

	// HandleTxCancel invoked on a request of transaction cancelation.
	HandleTxCancel(process *SagaProcess, id SagaTransactionID, reason string) SagaStatus

	//
	// Optional callbacks
	//

	// HandleDone invoked when the TX is done. Invoked on a saga where this tx was created.
	HandleTxDone(process *SagaProcess, id SagaTransactionID) SagaStatus

	// HandleInterim invoked if received interim result from the Next hop
	HandleTxInterim(process *SagaProcess, id SagaTransactionID, from SagaNextID, interim interface{}) SagaStatus

	//
	// Callbacks to handle result/interim from the worker(s)
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
	// HandleSagaDirect this callback is invoked on Process.Direct. This method is optional
	// for the implementation
	HandleSagaDirect(process *SagaProcess, message interface{}) (interface{}, error)
}

const (
	defaultHopLimit = math.MaxUint16
	defaultLifespan = 60
)

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
	// Lifespan defines a lifespan for the transaction in seconds. Must be > 0 (default is 60).
	Lifespan uint

	// TwoPhaseCommit enables 2PC for the transaction. This option makes all
	// Sagas involved in this transaction invoke HandleCommit callback on them and
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
	txs      map[SagaTransactionID]*SagaTransaction
	mutexTXS sync.Mutex

	// next sagas where txs were sent
	next      map[SagaNextID]SagaNext
	mutexNext sync.Mutex

	// running jobs
	jobs      map[SagaJobID]Process
	mutexJobs sync.Mutex
}

type SagaTransactionID etf.Ref

func (id SagaTransactionID) String() string {
	r := etf.Ref(id)
	return fmt.Sprintf("TX#%d.%d.%d", r.ID[0], r.ID[1], r.ID[2])
}

type SagaTransaction struct {
	id      SagaTransactionID
	options SagaTransactionOptions
	origin  SagaNextID // where it came from
	arrival int64      // when it came
	parents []etf.Pid

	context context.Context
	cancel  context.CancelFunc
}

type SagaNextID etf.Ref

func (id SagaNextID) String() string {
	r := etf.Ref(id)
	return fmt.Sprintf("Next#%d.%d.%d", r.ID[0], r.ID[1], r.ID[2])
}

type SagaNext struct {
	// Saga - etf.Pid, string (for the locally registered process), gen.ProcessID{process, node} (for the remote process)
	Saga interface{}
	// Value - a value for the invoking HandleTX on a Next hop.
	Value interface{}
	// Timeout - how long this Saga will be waiting for the result from the Next hop. Default - 10 seconds
	Timeout uint

	// internal
	id etf.Ref
	tx *SagaTransaction
}

type SagaJobID etf.Ref

func (id SagaJobID) String() string {
	r := etf.Ref(id)
	return fmt.Sprintf("Job#%d.%d.%d", r.ID[0], r.ID[1], r.ID[2])
}

type SagaJob struct {
	ID    SagaJobID
	Value interface{}

	// internal
	options SagaJobOptions
	commit  bool
	saga    etf.Pid
}

type SagaJobOptions struct {
	Timeout uint
}

type messageSaga struct {
	Request etf.Atom
	Pid     etf.Pid
	Command interface{}
}

type messageSagaNext struct {
	NextID        etf.Ref
	TransactionID etf.Ref
	Value         interface{}
	Parents       []etf.Pid
	Options       map[string]interface{}
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

func (sp *SagaProcess) StartTransaction(name string, options SagaTransactionOptions, value interface{}) SagaTransactionID {
	tx := SagaTransaction{
		id:      SagaTransactionID(sp.MakeRef()),
		arrival: time.Now().Unix(),
	}
	if options.HopLimit == 0 {
		options.HopLimit = defaultHopLimit
	}
	if options.Lifespan == 0 {
		options.Lifespan = defaultLifespan
	}
	tx.options = options

	sp.mutexTXS.Lock()
	sp.txs[tx.id] = &tx
	sp.mutexTXS.Unlock()

	next := SagaNext{
		Saga:  sp.Self(),
		Value: value,
		tx:    &tx,
	}
	sp.Next(tx.id, next)

	return tx.id

}

func (sp *SagaProcess) CancelTransaction(id SagaTransactionID, reason string) {
	sp.mutexTXS.Lock()
	tx, ok := sp.txs[id]
	sp.mutexTXS.Unlock()
	if !ok {
		return
	}

	message := etf.Tuple{
		etf.Atom("$saga_cancel"),
		sp.Self(),
		etf.Tuple{tx.id, etf.Ref(tx.origin), reason},
	}
	sp.Send(sp.Self(), message)
}

func (sp *SagaProcess) Next(id SagaTransactionID, next SagaNext) (SagaNextID, error) {
	sp.mutexTXS.Lock()
	tx, ok := sp.txs[id]
	sp.mutexTXS.Unlock()
	if !ok {
		return SagaNextID{}, fmt.Errorf("unknown transaction")
	}

	if tx.options.HopLimit == 0 {
		return SagaNextID{}, fmt.Errorf("exceeded hop limit")
	}

	next_id := SagaNextID(sp.MakeRef())
	next_HopLimit := tx.options.HopLimit - 1
	next_Lifespan := time.Now().Unix() - tx.arrival

	message := etf.Tuple{
		etf.Atom("$saga_next"),
		sp.Self(),
		etf.Tuple{
			etf.Ref(next_id),
			etf.Ref(tx.id),
			next.Value,
			tx.parents,
			etf.Map{
				"HopLimit":       next_HopLimit,
				"Lifespan":       next_Lifespan,
				"TwoPhaseCommit": tx.options.TwoPhaseCommit,
			},
		},
	}
	next.tx = tx
	sp.mutexNext.Lock()
	sp.next[next_id] = next
	sp.mutexNext.Unlock()

	// FIXME handle next.Timeout

	if err := sp.Send(next.Saga, message); err != nil {
		sp.CancelTransaction(tx.id, err.Error())
	}

	return next_id, nil
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
	job.commit = tx.options.TwoPhaseCommit

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
		txs:           make(map[SagaTransactionID]*SagaTransaction),
		next:          make(map[SagaNextID]SagaNext),
	}
	// do not inherite parent State
	sagaProcess.State = nil

	options, err := behavior.InitSaga(sagaProcess, args...)
	if err != nil {
		return err
	}

	sagaProcess.options = options
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

func (gs *Saga) HandleDirect(process *ServerProcess, message interface{}) (interface{}, error) {
	st := process.State.(*SagaProcess)
	switch m := message.(type) {
	case sagaSetMaxTransactions:
		st.options.MaxTransactions = m.max
		return nil, nil
	default:
		return process.Behavior().(SagaBehavior).HandleSagaDirect(st, message)
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
	//var cancel messageSagaCancel
	//var result messageSagaResult

	next := SagaNext{}
	switch m.Request {
	case etf.Atom("$saga_next"):
		if err := etf.TermIntoStruct(m.Command, &nextMessage); err != nil {
			return ErrUnsupportedRequest
		}

		// Check for the loop
		id := SagaTransactionID(nextMessage.TransactionID)
		tx, ok := process.txs[id]
		if ok && len(tx.parents) > 0 {
			cancel := etf.Tuple{
				etf.Atom("$saga_cancel"),
				process.Self(),
				etf.Tuple{
					nextMessage.NextID,
					nextMessage.TransactionID,
					"loop_detected",
				},
			}
			process.Send(m.Pid, cancel)
			return nil
		}

		// Check if exceed the number of transaction on this saga
		if process.options.MaxTransactions > 0 && len(process.txs)+1 > int(process.options.MaxTransactions) {
			cancel := etf.Tuple{
				etf.Atom("$saga_cancel"),
				process.Self(),
				etf.Tuple{
					nextMessage.NextID,
					nextMessage.TransactionID,
					"exceed_tx_limit",
				},
			}
			process.Send(m.Pid, cancel)
			return nil
		}

		//	// Check if lifespan is limited and transaction is too long
		//	lifespan := nextMessage.Transaction.options.Lifespan
		//	l := time.Now().Unix() - nextMessage.Transaction.arrived
		//	if lifespan > 0 && l > int64(lifespan) {
		//		cancel := etf.Tuple{
		//			etf.Atom("$saga_cancel"),
		//			process.Self(),
		//			etf.Tuple{
		//				nextMessage.Transaction.id,
		//				nextMessage.Ref,
		//				"exceed_lifespan",
		//			},
		//		}
		//		process.Send(m.Pid, cancel)
		//		return nil
		//	}

		// everything looks good. go further
		//process.txs[nextMessage.Transaction.id] = nextMessage.Transaction

		return process.Behavior().(SagaBehavior).HandleTxNew(process, id, next.Value)

		//case "$saga_cancel":
		//	if err := etf.TermIntoStruct(m.Command, &cancel); err != nil {
		//		return ErrUnsupportedRequest
		//	}
		//	tx, exist := process.txs[SagaTransactionID(cancel.ID)]
		//	if !exist {
		//		return nil
		//	}

		//	process.Behavior().(SagaBehavior).HandleTxCancel(process, tx.id, cancel.Reason)
		//	return nil
		//case "$saga_interim":
		//	if err := etf.TermIntoStruct(m.Command, &result); err != nil {
		//		return ErrUnsupportedRequest
		//	}
		//	process.Behavior().(SagaBehavior).HandleTxInterim(process, result.Transaction.id, next, result.Result)
		//	return nil
		//case "$saga_result":
		//	if err := etf.TermIntoStruct(m.Command, &result); err != nil {
		//		return ErrUnsupportedRequest
		//	}
		//	process.Behavior().(SagaBehavior).HandleTxResult(process, result.Transaction.id, next, result.Result)
		//	return nil
	}
	return sagaStatusUnsupported
}

func handleSagaDown(process *SagaProcess, down MessageDown) error {
	return nil
}

//
// default Saga callbacks
//

func (gs *Saga) HandleTxInterim(process *SagaProcess, tx SagaTransaction, interim interface{}) SagaStatus {
	fmt.Printf("HandleInterim: unhandled message %#v\n", tx)
	return nil
}
func (gs *Saga) HandleSagaCall(process *SagaProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	fmt.Printf("HandleSagaCall: unhandled message (from %#v) %#v\n", from, message)
	return etf.Atom("ok"), ServerStatusOK
}
func (gs *Saga) HandleSagaCast(process *SagaProcess, message etf.Term) ServerStatus {
	fmt.Printf("HandleSagaCast: unhandled message %#v\n", message)
	return ServerStatusOK
}
func (gs *Saga) HandleSagaInfo(process *SagaProcess, message etf.Term) ServerStatus {
	fmt.Printf("HandleSagaInfo: unhandled message %#v\n", message)
	return ServerStatusOK
}
func (gs *Saga) HandleJobResult(process *SagaProcess, id SagaJobID, result interface{}) SagaStatus {
	fmt.Printf("HandleJobResult: unhandled message %#v\n", result)
	return SagaStatusOK
}
func (gs *Saga) HandleJobInterim(process *SagaProcess, id SagaJobID, interim interface{}) SagaStatus {
	fmt.Printf("HandleJobInterim: unhandled message %#v\n", interim)
	return SagaStatusOK
}
func (gs *Saga) HandleJobFailed(process *SagaProcess, id SagaJobID) SagaStatus {
	fmt.Printf("HandleJobFailed: unhandled message %#v\n", id)
	return nil
}
