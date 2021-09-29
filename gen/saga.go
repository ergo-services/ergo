package gen

import (
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

	// HandleTxDone invoked when the transaction is done on a saga where it was created.
	HandleTxDone(process *SagaProcess, id SagaTransactionID) SagaStatus

	// HandleTxInterim invoked if received interim result from the next hop
	HandleTxInterim(process *SagaProcess, id SagaTransactionID, from SagaNextID, interim interface{}) SagaStatus

	// HandleTxCommit invoked if TwoPhaseCommit option is enabled for the given TX once the
	// final result was received by the Saga created this TX. All sagas involved in this
	// TX handling are making this call.
	HandleTxCommit(process *SagaProcess, id SagaTransactionID) SagaStatus

	//
	// Callbacks to handle result/interim from the worker(s)
	//

	// HandleJobResult
	HandleJobResult(process *SagaProcess, id SagaJobID, result interface{}) SagaStatus
	// HandleJobInterim
	HandleJobInterim(process *SagaProcess, id SagaJobID, interim interface{}) SagaStatus
	// HandleJobFailed
	HandleJobFailed(process *SagaProcess, id SagaJobID, reason string) SagaStatus

	//
	// Server's callbacks
	//

	// HandleStageCall this callback is invoked on ServerProcess.Call. This method is optional
	// for the implementation
	HandleSagaCall(process *SagaProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus)
	// HandleStageCast this callback is invoked on ServerProcess.Cast. This method is optional
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
	SagaStatusOK   SagaStatus // nil
	SagaStatusStop SagaStatus = fmt.Errorf("stop")

	// internal
	sagaStatusUnsupported SagaStatus = fmt.Errorf("unsupported")

	ErrSagaTxEndOfLifespan = fmt.Errorf("End of TX lifespan")
	ErrSagaTxNextTimeout   = fmt.Errorf("Next saga timeout")
	ErrSagaUnknown         = fmt.Errorf("Unknown saga")
	ErrSagaJobUnknown      = fmt.Errorf("Unknown job")
)

type Saga struct {
	Server
}

type SagaTransactionOptions struct {
	// HopLimit defines a number of hop within the transaction. Default limit
	// is 0 (no limit).
	HopLimit uint
	// Lifespan defines a lifespan for the transaction in seconds. Default is 60.
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
	options  SagaOptions
	behavior SagaBehavior

	// running transactions
	txs      map[SagaTransactionID]*SagaTransaction
	mutexTXS sync.Mutex

	// next sagas where txs were sent
	next      map[SagaNextID]*SagaTransaction
	mutexNext sync.Mutex

	// running jobs
	jobs      map[etf.Pid]*SagaJob
	mutexJobs sync.Mutex
}

type SagaTransactionID etf.Ref

func (id SagaTransactionID) String() string {
	r := etf.Ref(id)
	return fmt.Sprintf("TX#%d.%d.%d", r.ID[0], r.ID[1], r.ID[2])
}

type SagaTransaction struct {
	sync.Mutex
	id      SagaTransactionID
	options SagaTransactionOptions
	origin  SagaNextID               // where it came from
	monitor etf.Ref                  // monitor parent process
	next    map[SagaNextID]*SagaNext // where were sent
	jobs    map[etf.Pid]bool
	arrival int64     // when it arrived on this saga
	parents []etf.Pid // sagas trace

	done bool // do not allow send result more than once if 2PC is set
}

type SagaNextID etf.Ref

func (id SagaNextID) String() string {
	r := etf.Ref(id)
	return fmt.Sprintf("Next#%d.%d.%d", r.ID[0], r.ID[1], r.ID[2])
}

type SagaNext struct {
	// Saga etf.Pid, string (for the locally registered process), gen.ProcessID{process, node} (for the remote process)
	Saga interface{}
	// Value a value for the invoking HandleTX on a next hop.
	Value interface{}
	// Timeout how long this Saga will be waiting for the result from the next hop. Default - 10 seconds
	Timeout uint
	// TrapCancel if the next saga fails, it will transform the cancel signal into the regular message gen.MessageSagaCancel, and HandleSagaInfo callback will be invoked.
	TrapCancel bool

	// internal
	done bool // for 2PC case
}

type SagaJobID etf.Ref

func (id SagaJobID) String() string {
	r := etf.Ref(id)
	return fmt.Sprintf("Job#%d.%d.%d", r.ID[0], r.ID[1], r.ID[2])
}

type SagaJob struct {
	ID            SagaJobID
	TransactionID SagaTransactionID
	Value         interface{}

	// internal
	options SagaJobOptions
	saga    etf.Pid
	commit  bool
	worker  Process
	done    bool
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
	TransactionID etf.Ref
	Origin        etf.Ref
	Value         interface{}
	Parents       []etf.Pid
	Options       map[string]interface{}
}

type messageSagaResult struct {
	TransactionID etf.Ref
	Origin        etf.Ref
	Result        interface{}
}

type messageSagaCancel struct {
	Transaction etf.Ref
	Origin      etf.Ref
	Reason      string
}

type MessageSagaCancel struct {
	TransactionID SagaTransactionID
	NextID        SagaNextID
	Reason        string
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
	id := sp.MakeRef()

	if options.HopLimit == 0 {
		options.HopLimit = defaultHopLimit
	}
	if options.Lifespan == 0 {
		options.Lifespan = defaultLifespan
	}

	message := etf.Tuple{
		etf.Atom("$saga_next"),
		sp.Self(),
		etf.Tuple{
			id,          // tx id
			etf.Ref{},   // origin. empty value. (parent's next id)
			value,       // tx value
			[]etf.Pid{}, // parents
			etf.Map{ // tx options
				"HopLimit":       options.HopLimit,
				"Lifespan":       options.Lifespan,
				"TwoPhaseCommit": options.TwoPhaseCommit,
			},
		},
	}

	sp.Send(sp.Self(), message)
	return SagaTransactionID(id)
}

func (sp *SagaProcess) Next(id SagaTransactionID, next SagaNext) (SagaNextID, error) {
	sp.mutexTXS.Lock()
	tx, ok := sp.txs[id]
	sp.mutexTXS.Unlock()
	if !ok {
		return SagaNextID{}, fmt.Errorf("unknown transaction")
	}

	if len(tx.next) > int(tx.options.HopLimit) {
		return SagaNextID{}, fmt.Errorf("exceeded hop limit")
	}

	next_Lifespan := int64(tx.options.Lifespan) - (time.Now().Unix() - tx.arrival)
	if next_Lifespan < 1 {
		sp.CancelTransaction(id, "exceeded lifespan")
		return SagaNextID{}, fmt.Errorf("exceeded lifespan. transaction canceled")
	}

	ref := sp.MonitorProcess(next.Saga)
	next_id := SagaNextID(ref)
	message := etf.Tuple{
		etf.Atom("$saga_next"),
		sp.Self(),
		etf.Tuple{
			etf.Ref(tx.id), // tx id
			ref,            // next id
			next.Value,
			tx.parents,
			etf.Map{
				"HopLimit":       tx.options.HopLimit,
				"Lifespan":       next_Lifespan,
				"TwoPhaseCommit": tx.options.TwoPhaseCommit,
			},
		},
	}

	sp.Send(next.Saga, message)

	tx.Lock()
	tx.next[next_id] = &next
	tx.Unlock()

	sp.mutexNext.Lock()
	sp.next[next_id] = tx
	sp.mutexNext.Unlock()

	// FIXME handle next.Timeout

	return next_id, nil
}

func (sp *SagaProcess) StartJob(id SagaTransactionID, options SagaJobOptions, value interface{}) (SagaJobID, error) {
	var job SagaJob

	if sp.options.Worker == nil {
		return job.ID, fmt.Errorf("This saga has no worker")
	}
	sp.mutexTXS.Lock()
	tx, ok := sp.txs[id]
	sp.mutexTXS.Unlock()

	if !ok {
		return job.ID, fmt.Errorf("unknown transaction")
	}

	// FIXME make context WithTimeout to limit the lifespan
	workerOptions := ProcessOptions{}
	worker, err := sp.Spawn("", workerOptions, sp.options.Worker)
	if err != nil {
		return job.ID, err
	}
	sp.Link(worker.Self())

	job.ID = SagaJobID(sp.MakeRef())
	job.TransactionID = id
	job.Value = value
	job.commit = tx.options.TwoPhaseCommit
	job.saga = sp.Self()
	job.worker = worker

	m := messageSagaJobStart{
		job: job,
	}
	if err := sp.Cast(worker.Self(), m); err != nil {
		worker.Kill()
		return SagaJobID{}, err
	}

	sp.mutexJobs.Lock()
	sp.jobs[worker.Self()] = &job
	sp.mutexJobs.Unlock()

	tx.Lock()
	tx.jobs[worker.Self()] = true
	tx.Unlock()

	return job.ID, nil
}

func (sp *SagaProcess) SendResult(id SagaTransactionID, result interface{}) error {
	sp.mutexTXS.Lock()
	tx, ok := sp.txs[id]
	sp.mutexTXS.Unlock()
	if !ok {
		return fmt.Errorf("unknown transaction")
	}
	if tx.done {
		return fmt.Errorf("result is already sent")
	}

	if sp.checkTxDone(tx) == false {
		return fmt.Errorf("transaction is still in progress")
	}

	if len(tx.parents) == 0 {
		// SendResult is called right after CreateTransaction call.
		return fmt.Errorf("not allowed")
	}

	message := etf.Tuple{
		etf.Atom("$saga_result"),
		sp.Self(),
		etf.Tuple{
			etf.Ref(tx.id),
			etf.Ref(tx.origin),
			result,
		},
	}

	// send message to the parent saga
	if err := sp.Cast(tx.parents[0], message); err != nil {
		return err
	}

	// tx handling is done on this saga
	tx.done = true

	// do not remove TX if we send result to itself
	if tx.parents[0] == sp.Self() {
		return nil
	}

	// do not remove TX if 2PC is enabled
	if tx.options.TwoPhaseCommit {
		return nil
	}

	sp.mutexTXS.Lock()
	delete(sp.txs, id)
	sp.mutexTXS.Unlock()

	return nil
}

func (sp *SagaProcess) SendInterim(id SagaTransactionID, interim interface{}) error {
	sp.mutexTXS.Lock()
	tx, ok := sp.txs[id]
	sp.mutexTXS.Unlock()
	if !ok {
		return fmt.Errorf("unknown transaction")
	}

	message := etf.Tuple{
		etf.Atom("$saga_interim"),
		sp.Self(),
		etf.Tuple{
			etf.Ref(tx.id),
			etf.Ref(tx.origin),
			interim,
		},
	}

	// send message to the parent saga
	if err := sp.Cast(tx.parents[0], message); err != nil {
		return err
	}

	return nil
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

func (sp *SagaProcess) CancelJob(job SagaJobID) error {
	return nil

}

func (sp *SagaProcess) checkTxDone(tx *SagaTransaction) bool {

	if tx.options.TwoPhaseCommit == false { // 2PC is disabled
		if len(tx.next) > 0 { // haven't received all results from the "next" sagas
			return false
		}
		if len(tx.jobs) > 0 { // tx has running jobs
			return false
		}
		return true
	}

	// 2PC is enabled. check whether received all results from sagas
	// and workers have finished their jobs

	tx.Lock()
	// check results from sagas
	for _, next := range tx.next {
		if next.done == false {
			tx.Unlock()
			return false
		}
	}

	if len(tx.jobs) == 0 {
		tx.Unlock()
		return true
	}

	// gen list of running workers
	jobs := []etf.Pid{}
	for pid, _ := range tx.jobs {
		jobs = append(jobs, pid)
	}
	tx.Unlock()

	// check the job states of them
	sp.mutexJobs.Lock()
	for _, pid := range jobs {
		job := sp.jobs[pid]
		if job.done == false {
			sp.mutexJobs.Unlock()
			return false
		}
	}
	sp.mutexJobs.Unlock()
	return true
}

func (sp *SagaProcess) handleSagaRequest(m messageSaga) error {

	switch m.Request {
	case etf.Atom("$saga_next"):
		nextMessage := messageSagaNext{}

		if err := etf.TermIntoStruct(m.Command, &nextMessage); err != nil {
			return ErrUnsupportedRequest
		}

		// Check if exceed the number of transaction on this saga
		if sp.options.MaxTransactions > 0 && len(sp.txs)+1 > int(sp.options.MaxTransactions) {
			cancel := etf.Tuple{
				etf.Atom("$saga_cancel"),
				sp.Self(),
				etf.Tuple{
					nextMessage.TransactionID,
					nextMessage.Origin,
					"exceed_tx_limit",
				},
			}
			sp.Send(m.Pid, cancel)
			return nil
		}

		// Check for the loop
		transactionID := SagaTransactionID(nextMessage.TransactionID)
		sp.mutexTXS.Lock()
		tx, ok := sp.txs[transactionID]
		sp.mutexTXS.Unlock()
		if ok {
			// loop detected. send cancel message
			cancel := etf.Tuple{
				etf.Atom("$saga_cancel"),
				sp.Self(),
				etf.Tuple{
					nextMessage.TransactionID,
					nextMessage.Origin,
					"loop_detected",
				},
			}
			sp.Send(m.Pid, cancel)
			return nil
		}

		txOptions := SagaTransactionOptions{
			HopLimit: defaultHopLimit,
			Lifespan: defaultLifespan,
		}
		if value, ok := nextMessage.Options["HopLimit"]; ok {
			if hoplimit, ok := value.(int64); ok {
				txOptions.HopLimit = uint(hoplimit)
			}
		}
		if value, ok := nextMessage.Options["Lifespan"]; ok {
			if lifespan, ok := value.(int64); ok {
				txOptions.Lifespan = uint(lifespan)
			}
		}
		if value, ok := nextMessage.Options["TwoPhaseCommit"]; ok {
			txOptions.TwoPhaseCommit, _ = value.(bool)
		}

		tx = &SagaTransaction{
			id:      transactionID,
			options: txOptions,
			origin:  SagaNextID(nextMessage.Origin),
			next:    make(map[SagaNextID]*SagaNext),
			jobs:    make(map[etf.Pid]bool),
			parents: append([]etf.Pid{m.Pid}, nextMessage.Parents...),
		}
		sp.mutexTXS.Lock()
		sp.txs[transactionID] = tx
		sp.mutexTXS.Unlock()

		// do not monitor itself (they are equal if its came from the StartTransaction call)
		if m.Pid != sp.Self() {
			tx.monitor = sp.MonitorProcess(m.Pid)
		}
		// FIXME start lifespan timer
		return sp.behavior.HandleTxNew(sp, transactionID, nextMessage.Value)

	//case "$saga_cancel":
	//	if err := etf.TermIntoStruct(m.Command, &cancel); err != nil {
	//		return ErrUnsupportedRequest
	//	}
	//	tx, exist := sp.txs[SagaTransactionID(cancel.ID)]
	//	if !exist {
	//		return nil
	//	}

	//	sp.Behavior().(SagaBehavior).HandleTxCancel(sp, tx.id, cancel.Reason)
	//	return nil
	case etf.Atom("$saga_result"):
		result := messageSagaResult{}
		if err := etf.TermIntoStruct(m.Command, &result); err != nil {
			return ErrUnsupportedRequest
		}

		transactionID := SagaTransactionID(result.TransactionID)
		sp.mutexTXS.Lock()
		tx, ok := sp.txs[transactionID]
		sp.mutexTXS.Unlock()
		if !ok {
			// ignore unknown TX
			return nil
		}

		next_id := SagaNextID(result.Origin)
		empty_next_id := SagaNextID{}
		// next id is empty if we got result on a saga created this TX
		if next_id != empty_next_id {
			sp.mutexNext.Lock()
			_, ok := sp.next[next_id]
			sp.mutexNext.Unlock()
			if !ok {
				// ignore unknown result
				return nil
			}
			sp.mutexNext.Lock()
			delete(sp.next, next_id)
			sp.mutexNext.Unlock()

			tx.Lock()
			if tx.options.TwoPhaseCommit == false {
				delete(tx.next, next_id)
			} else {
				next := tx.next[next_id]
				next.done = true
			}
			tx.Unlock()

			return sp.behavior.HandleTxResult(sp, tx.id, next_id, result.Result)
		}

		status := sp.behavior.HandleTxResult(sp, tx.id, next_id, result.Result)
		if sp.checkTxDone(tx) == true {
			return sp.behavior.HandleTxDone(sp, tx.id)
		}

		return status

	case etf.Atom("$saga_interim"):
		interim := messageSagaResult{}
		if err := etf.TermIntoStruct(m.Command, &interim); err != nil {
			return ErrUnsupportedRequest
		}
		next_id := SagaNextID(interim.Origin)
		sp.mutexNext.Lock()
		tx, ok := sp.next[next_id]
		sp.mutexNext.Unlock()
		if !ok {
			// ignore unknown interim result
			return nil
		}
		sp.behavior.HandleTxInterim(sp, tx.id, next_id, interim.Result)
		return nil
	}
	return sagaStatusUnsupported
}

func (sp *SagaProcess) handleSagaExit(exit MessageExit) error {
	sp.mutexJobs.Lock()
	job, ok := sp.jobs[exit.Pid]
	sp.mutexJobs.Unlock()
	if !ok {
		// passthrough this message to HandleSagaInfo callback
		return ErrSagaJobUnknown
	}

	// remove it
	sp.mutexJobs.Lock()
	delete(sp.jobs, exit.Pid)
	sp.mutexJobs.Unlock()

	// check if this tx is still alive
	sp.mutexTXS.Lock()
	_, ok = sp.txs[job.TransactionID]
	sp.mutexTXS.Unlock()
	if !ok {
		// seems it was already canceled
		return SagaStatusOK
	}

	// if this job is done, don't care about the termination reason
	if job.done {
		return SagaStatusOK
	}

	if exit.Reason != "normal" {
		return sp.behavior.HandleJobFailed(sp, job.ID, exit.Reason)
	}

	// seems no result received from this worker
	return sp.behavior.HandleJobFailed(sp, job.ID, "no result")
}

func (sp *SagaProcess) handleSagaDown(down MessageDown) error {
	// FIXME

	// if down.Ref is unknown return ErrSagaUnknown to passthrough
	// this message to HandleSagaInfo callback

	// if down.Ref == Origin - parent saga is down

	// check the sp.next for this down.Ref
	// if found - next saga is down. send cancel message to itself
	// with reason "next saga down"
	return nil
}

//
// Server callbacks
//

func (gs *Saga) Init(process *ServerProcess, args ...etf.Term) error {
	var options SagaOptions

	behavior, ok := process.Behavior().(SagaBehavior)
	if !ok {
		return fmt.Errorf("Saga: not a SagaBehavior")
	}

	sagaProcess := &SagaProcess{
		ServerProcess: *process,
		txs:           make(map[SagaTransactionID]*SagaTransaction),
		next:          make(map[SagaNextID]*SagaTransaction),
		behavior:      behavior,
	}
	// do not inherit parent State
	sagaProcess.State = nil

	options, err := behavior.InitSaga(sagaProcess, args...)
	if err != nil {
		return err
	}

	sagaProcess.options = options
	process.State = sagaProcess

	if options.Worker != nil {
		sagaProcess.jobs = make(map[etf.Pid]*SagaJob)
	}

	process.SetTrapExit(true)

	return nil
}

func (gs *Saga) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	sp := process.State.(*SagaProcess)
	return sp.behavior.HandleSagaCall(sp, from, message)
}

func (gs *Saga) HandleDirect(process *ServerProcess, message interface{}) (interface{}, error) {
	sp := process.State.(*SagaProcess)
	switch m := message.(type) {
	case sagaSetMaxTransactions:
		sp.options.MaxTransactions = m.max
		return nil, nil
	default:
		return sp.behavior.HandleSagaDirect(sp, message)
	}
}

func (gs *Saga) HandleCast(process *ServerProcess, message etf.Term) ServerStatus {
	var status SagaStatus
	var mSaga messageSaga

	sp := process.State.(*SagaProcess)

	switch m := message.(type) {
	case messageSagaJobResult:
		sp.mutexJobs.Lock()
		job, ok := sp.jobs[m.pid]
		sp.mutexJobs.Unlock()
		if !ok {
			status = SagaStatusOK
			break
		}

		sp.mutexTXS.Lock()
		tx, ok := sp.txs[job.TransactionID]
		sp.mutexTXS.Unlock()

		if !ok {
			// might be already canceled. just ignore it
			status = SagaStatusOK
			break
		}
		job.done = true

		// remove this job from the tx job list, but do not remove
		// from the sp.jobs (will be removed once worker terminated)
		if tx.options.TwoPhaseCommit == false {
			tx.Lock()
			delete(tx.jobs, m.pid)
			tx.Unlock()
		}

		status = sp.behavior.HandleJobResult(sp, job.ID, m.result)

	case messageSagaJobInterim:
		sp.mutexJobs.Lock()
		job, ok := sp.jobs[m.pid]
		sp.mutexJobs.Unlock()
		if !ok {
			// might be already canceled. just ignore it
			status = SagaStatusOK
			break
		}
		status = sp.behavior.HandleJobInterim(sp, job.ID, m.interim)

	default:
		if err := etf.TermIntoStruct(message, &mSaga); err == nil {
			// handle Interim and Result messages comes from the "next" sagas
			s := sp.handleSagaRequest(mSaga)
			status = ServerStatus(s)
			break
		}

		status = sp.behavior.HandleSagaCast(sp, message)
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
	var mSaga messageSaga

	sp := process.State.(*SagaProcess)
	switch m := message.(type) {
	case MessageExit:
		// handle worker exit message
		err := sp.handleSagaExit(m)
		if err == ErrSagaJobUnknown {
			return sp.behavior.HandleSagaInfo(sp, m)
		}
		return ServerStatus(err)

	case MessageDown:
		// handle saga's down message
		err := sp.handleSagaDown(m)
		if err == ErrSagaUnknown {
			return sp.behavior.HandleSagaInfo(sp, m)
		}
		return ServerStatus(err)

	default:
		if err := etf.TermIntoStruct(message, &mSaga); err != nil {
			status := sp.behavior.HandleSagaInfo(sp, message)
			return status
		}
	}

	status := sp.handleSagaRequest(mSaga)
	switch status {
	case nil, SagaStatusOK:
		return ServerStatusOK
	case SagaStatusStop:
		return ServerStatusStop
	case sagaStatusUnsupported:
		return sp.behavior.HandleSagaInfo(sp, message)
	default:
		return ServerStatus(status)
	}
}

func (gs *Saga) Terminate(process *ServerProcess, reason string) {
	fmt.Println("SAGA terminated")
}

//
// default Saga callbacks
//

func (gs *Saga) HandleTxInterim(process *SagaProcess, id SagaTransactionID, from SagaNextID, interim interface{}) SagaStatus {
	fmt.Printf("HandleTxInterim: unhandled message %v from %v\n", id, from)
	return ServerStatusOK
}
func (gs *Saga) HandleTxCommit(process *SagaProcess, id SagaTransactionID) SagaStatus {
	fmt.Printf("HandleTxCommit: unhandled message %v\n", id)
	return ServerStatusOK
}
func (gs *Saga) HandleTxDone(process *SagaProcess, id SagaTransactionID) SagaStatus {
	fmt.Printf("HandleTxDone: unhandled message %v\n", id)
	return ServerStatusOK
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
func (gs *Saga) HandleJobFailed(process *SagaProcess, id SagaJobID, reason string) SagaStatus {
	fmt.Printf("HandleJobFailed: unhandled message %s. reason %q\n", id, reason)
	return nil
}
