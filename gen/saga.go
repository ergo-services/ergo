package gen

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ergo-services/ergo/etf"
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
	// It returns the final result and SagaStatus. The commit message will deliver the final
	// result to all participants of this transaction (if it has enabled the TwoPhaseCommit option).
	// Otherwise the final result will be ignored.
	HandleTxDone(process *SagaProcess, id SagaTransactionID, result interface{}) (interface{}, SagaStatus)

	// HandleTxInterim invoked if received interim result from the next hop
	HandleTxInterim(process *SagaProcess, id SagaTransactionID, from SagaNextID, interim interface{}) SagaStatus

	// HandleTxCommit invoked if TwoPhaseCommit option is enabled for the given TX.
	// All sagas involved in this TX receive a commit message with final value and invoke this callback.
	// The final result has a value returned by HandleTxDone on a Saga created this TX.
	HandleTxCommit(process *SagaProcess, id SagaTransactionID, final interface{}) SagaStatus

	//
	// Callbacks to handle result/interim from the worker(s)
	//

	// HandleJobResult
	HandleJobResult(process *SagaProcess, id SagaTransactionID, from SagaJobID, result interface{}) SagaStatus
	// HandleJobInterim
	HandleJobInterim(process *SagaProcess, id SagaTransactionID, from SagaJobID, interim interface{}) SagaStatus
	// HandleJobFailed
	HandleJobFailed(process *SagaProcess, id SagaTransactionID, from SagaJobID, reason string) SagaStatus

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

	ErrSagaTxEndOfLifespan   = fmt.Errorf("End of TX lifespan")
	ErrSagaTxNextTimeout     = fmt.Errorf("Next saga timeout")
	ErrSagaUnknown           = fmt.Errorf("Unknown saga")
	ErrSagaJobUnknown        = fmt.Errorf("Unknown job")
	ErrSagaTxUnknown         = fmt.Errorf("Unknown TX")
	ErrSagaTxCanceled        = fmt.Errorf("Tx is canceled")
	ErrSagaTxInProgress      = fmt.Errorf("Tx is still in progress")
	ErrSagaResultAlreadySent = fmt.Errorf("Result is already sent")
	ErrSagaNotAllowed        = fmt.Errorf("Operation is not allowed")
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
	origin  SagaNextID               // next id on a saga it came from
	monitor etf.Ref                  // monitor parent saga
	next    map[SagaNextID]*SagaNext // where were sent
	jobs    map[SagaJobID]etf.Pid
	arrival int64     // when it arrived on this saga
	parents []etf.Pid // sagas trace

	done        bool // do not allow send result more than once if 2PC is set
	cancelTimer context.CancelFunc
}

type SagaNextID etf.Ref

func (id SagaNextID) String() string {
	r := etf.Ref(id)
	return fmt.Sprintf("Next#%d.%d.%d", r.ID[0], r.ID[1], r.ID[2])
}

type SagaNext struct {
	// Saga etf.Pid, string (for the locally registered process), gen.ProcessID{process, node} (for the remote process)
	Saga interface{}
	// Value a value for the invoking HandleTxNew on a next hop.
	Value interface{}
	// Timeout how long this Saga will be waiting for the result from the next hop. Default - 10 seconds
	Timeout uint
	// TrapCancel if the next saga fails, it will transform the cancel signal into the regular message gen.MessageSagaCancel, and HandleSagaInfo callback will be invoked.
	TrapCancel bool

	// internal
	done        bool // for 2PC case
	cancelTimer context.CancelFunc
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
	options     SagaJobOptions
	saga        etf.Pid
	commit      bool
	worker      Process
	done        bool
	cancelTimer context.CancelFunc
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
	TransactionID etf.Ref
	Origin        etf.Ref
	Reason        string
}

type messageSagaCommit struct {
	TransactionID etf.Ref
	Origin        etf.Ref
	Final         interface{}
}

type MessageSagaCancel struct {
	TransactionID SagaTransactionID
	NextID        SagaNextID
	Reason        string
}

type MessageSagaError struct {
	TransactionID SagaTransactionID
	NextID        SagaNextID
	Error         string
	Details       string
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

func (sp *SagaProcess) StartTransaction(options SagaTransactionOptions, value interface{}) SagaTransactionID {
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
		return SagaNextID{}, ErrSagaTxUnknown
	}

	if len(tx.next) > int(tx.options.HopLimit) {
		return SagaNextID{}, fmt.Errorf("exceeded hop limit")
	}

	nextLifespan := int64(tx.options.Lifespan) - (time.Now().Unix() - tx.arrival)
	if nextLifespan < 1 {
		sp.CancelTransaction(id, "exceeded lifespan")
		return SagaNextID{}, fmt.Errorf("exceeded lifespan. transaction canceled")
	}

	if next.Timeout > 0 && int64(next.Timeout) > nextLifespan {
		return SagaNextID{}, fmt.Errorf("requested timeout exceed lifespan")
	}

	if next.Timeout > 0 {
		nextLifespan = int64(next.Timeout)
	}

	ref := sp.MonitorProcess(next.Saga)
	next_id := SagaNextID(ref)
	message := etf.Tuple{
		etf.Atom("$saga_next"),
		sp.Self(),
		etf.Tuple{
			etf.Ref(tx.id), // tx id
			ref,            // next id (tx origin on the next saga)
			next.Value,
			tx.parents,
			etf.Map{
				"HopLimit":       tx.options.HopLimit,
				"Lifespan":       nextLifespan,
				"TwoPhaseCommit": tx.options.TwoPhaseCommit,
			},
		},
	}

	sp.Send(next.Saga, message)

	cancelMessage := etf.Tuple{
		etf.Atom("$saga_cancel"),
		etf.Pid{}, // do not send sp.Self() to be able TrapCancel work
		etf.Tuple{
			etf.Ref(tx.id), // tx id
			ref,
			"lifespan",
		},
	}
	timeout := time.Duration(nextLifespan) * time.Second
	next.cancelTimer = sp.SendAfter(sp.Self(), cancelMessage, timeout)

	tx.Lock()
	tx.next[next_id] = &next
	tx.Unlock()

	sp.mutexNext.Lock()
	sp.next[next_id] = tx
	sp.mutexNext.Unlock()

	return next_id, nil
}

func (sp *SagaProcess) StartJob(id SagaTransactionID, options SagaJobOptions, value interface{}) (SagaJobID, error) {

	if sp.options.Worker == nil {
		return SagaJobID{}, fmt.Errorf("This saga has no worker")
	}
	sp.mutexTXS.Lock()
	tx, ok := sp.txs[id]
	sp.mutexTXS.Unlock()

	if !ok {
		return SagaJobID{}, ErrSagaTxUnknown
	}

	jobLifespan := int64(tx.options.Lifespan) - (time.Now().Unix() - tx.arrival)
	if options.Timeout > 0 && int64(options.Timeout) > jobLifespan {
		return SagaJobID{}, fmt.Errorf("requested timeout exceed lifespan")
	}
	if options.Timeout > 0 {
		jobLifespan = int64(options.Timeout)
	}

	workerOptions := ProcessOptions{}
	worker, err := sp.Spawn("", workerOptions, sp.options.Worker)
	if err != nil {
		return SagaJobID{}, err
	}
	sp.Link(worker.Self())

	job := SagaJob{
		ID:            SagaJobID(sp.MakeRef()),
		TransactionID: id,
		Value:         value,
		commit:        tx.options.TwoPhaseCommit,
		saga:          sp.Self(),
		worker:        worker,
	}

	sp.mutexJobs.Lock()
	sp.jobs[worker.Self()] = &job
	sp.mutexJobs.Unlock()

	m := messageSagaJobStart{
		job: job,
	}
	tx.Lock()
	tx.jobs[job.ID] = worker.Self()
	tx.Unlock()

	sp.Cast(worker.Self(), m)

	// terminate worker process via handleSagaExit
	exitMessage := MessageExit{
		Pid:    worker.Self(),
		Reason: "lifespan",
	}

	timeout := time.Duration(jobLifespan) * time.Second
	job.cancelTimer = sp.SendAfter(sp.Self(), exitMessage, timeout)

	return job.ID, nil
}

func (sp *SagaProcess) SendResult(id SagaTransactionID, result interface{}) error {
	sp.mutexTXS.Lock()
	tx, ok := sp.txs[id]
	sp.mutexTXS.Unlock()
	if !ok {
		return ErrSagaTxUnknown
	}

	if len(tx.parents) == 0 {
		// SendResult was called right after CreateTransaction call.
		return ErrSagaNotAllowed
	}

	if tx.done {
		return ErrSagaResultAlreadySent
	}

	if sp.checkTxDone(tx) == false {
		return ErrSagaTxInProgress
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
	if err := sp.Send(tx.parents[0], message); err != nil {
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
		return ErrSagaTxUnknown
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
	if err := sp.Send(tx.parents[0], message); err != nil {
		return err
	}

	return nil
}

func (sp *SagaProcess) CancelTransaction(id SagaTransactionID, reason string) error {
	sp.mutexTXS.Lock()
	tx, ok := sp.txs[id]
	sp.mutexTXS.Unlock()
	if !ok {
		return ErrSagaTxUnknown
	}

	message := etf.Tuple{
		etf.Atom("$saga_cancel"),
		sp.Self(),
		etf.Tuple{etf.Ref(tx.id), etf.Ref(tx.origin), reason},
	}
	sp.Send(sp.Self(), message)
	return nil
}

func (sp *SagaProcess) CancelJob(id SagaTransactionID, job SagaJobID, reason string) error {
	sp.mutexTXS.Lock()
	tx, ok := sp.txs[id]
	sp.mutexTXS.Unlock()
	if !ok {
		return ErrSagaTxUnknown
	}
	tx.Lock()
	defer tx.Unlock()
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
	for _, pid := range tx.jobs {
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
			if lifespan, ok := value.(int64); ok && lifespan > 0 {
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
			jobs:    make(map[SagaJobID]etf.Pid),
			arrival: time.Now().Unix(),
			parents: append([]etf.Pid{m.Pid}, nextMessage.Parents...),
		}
		sp.mutexTXS.Lock()
		sp.txs[transactionID] = tx
		sp.mutexTXS.Unlock()

		// do not monitor itself (they are equal if its came from the StartTransaction call)
		if m.Pid != sp.Self() {
			tx.monitor = sp.MonitorProcess(m.Pid)
		}

		// tx lifespan timer
		cancelMessage := etf.Tuple{
			etf.Atom("$saga_cancel"),
			sp.Self(), // can't be trapped (ignored)
			etf.Tuple{
				nextMessage.TransactionID,
				nextMessage.Origin,
				"lifespan",
			},
		}
		timeout := time.Duration(txOptions.Lifespan) * time.Second
		tx.cancelTimer = sp.SendAfter(sp.Self(), cancelMessage, timeout)

		return sp.behavior.HandleTxNew(sp, transactionID, nextMessage.Value)

	case "$saga_cancel":
		cancel := messageSagaCancel{}
		if err := etf.TermIntoStruct(m.Command, &cancel); err != nil {
			return ErrUnsupportedRequest
		}

		tx, exist := sp.txs[SagaTransactionID(cancel.TransactionID)]
		if !exist {
			// unknown tx, just ignore it
			return nil
		}

		// check where it came from.
		if tx.parents[0] == m.Pid {
			// came from parent saga or from itself via CancelTransaction
			// can't be ignored
			sp.cancelTX(m.Pid, cancel, tx)
			return sp.behavior.HandleTxCancel(sp, tx.id, cancel.Reason)
		}

		// this cancel came from one of the next sagas
		// or from itself (being in the middle of transaction graph)
		next_id := SagaNextID(cancel.Origin)
		tx.Lock()
		next, ok := tx.next[next_id]
		tx.Unlock()

		if ok && next.TrapCancel {
			// clean the next saga stuff
			next.cancelTimer()
			sp.DemonitorProcess(cancel.Origin)
			tx.Lock()
			delete(tx.next, next_id)
			tx.Unlock()
			sp.mutexNext.Lock()
			delete(sp.next, next_id)
			sp.mutexNext.Unlock()

			// came from the next saga and TrapCancel was enabled
			cm := MessageSagaCancel{
				TransactionID: tx.id,
				NextID:        next_id,
				Reason:        cancel.Reason,
			}
			sp.Send(sp.Self(), cm)
			return SagaStatusOK
		}

		sp.cancelTX(m.Pid, cancel, tx)
		return sp.behavior.HandleTxCancel(sp, tx.id, cancel.Reason)

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
			next := tx.next[next_id]
			if tx.options.TwoPhaseCommit == false {
				next.cancelTimer()
				sp.DemonitorProcess(result.Origin)
				delete(tx.next, next_id)
			} else {
				next.done = true
			}
			tx.Unlock()

			return sp.behavior.HandleTxResult(sp, tx.id, next_id, result.Result)
		}

		final, status := sp.behavior.HandleTxDone(sp, tx.id, result.Result)
		if status == SagaStatusOK {
			sp.commitTX(tx, final)
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
			// ignore unknown interim result and send cancel message to the sender
			message := etf.Tuple{
				etf.Atom("$saga_cancel"),
				sp.Self(),
				etf.Tuple{
					interim.TransactionID,
					interim.Origin,
					"unknown or canceled tx",
				},
			}
			sp.Send(m.Pid, message)
			return nil
		}
		return sp.behavior.HandleTxInterim(sp, tx.id, next_id, interim.Result)

	case etf.Atom("$saga_commit"):
		// propagate Commit signal if 2PC is enabled
		commit := messageSagaCommit{}
		if err := etf.TermIntoStruct(m.Command, &commit); err != nil {
			return ErrUnsupportedRequest
		}
		transactionID := SagaTransactionID(commit.TransactionID)
		sp.mutexTXS.Lock()
		tx, ok := sp.txs[transactionID]
		sp.mutexTXS.Unlock()
		if !ok {
			// ignore unknown TX
			return nil
		}
		// clean up and send commit message before we invoke callback
		sp.commitTX(tx, commit.Final)
		// make sure if 2PC was enabled on this TX
		if tx.options.TwoPhaseCommit {
			return sp.behavior.HandleTxCommit(sp, tx.id, commit.Final)
		}
		return SagaStatusOK
	}
	return sagaStatusUnsupported
}

func (sp *SagaProcess) cancelTX(from etf.Pid, cancel messageSagaCancel, tx *SagaTransaction) {
	tx.cancelTimer()

	// stop workers
	tx.Lock()
	cancelJobs := []etf.Pid{}
	for _, pid := range tx.jobs {
		sp.Unlink(pid)
		sp.Cast(pid, messageSagaJobCancel{reason: cancel.Reason})
		cancelJobs = append(cancelJobs, pid)
	}
	tx.Unlock()

	sp.mutexJobs.Lock()
	for i := range cancelJobs {
		job, ok := sp.jobs[cancelJobs[i]]
		if ok {
			delete(sp.jobs, cancelJobs[i])
			job.cancelTimer()
		}
	}
	sp.mutexJobs.Unlock()

	// remove monitor from parent saga
	if tx.parents[0] != sp.Self() {
		sp.DemonitorProcess(tx.monitor)

		// do not send to the parent saga if it came from there
		// and cancelation reason caused by lifespan timer
		if tx.parents[0] != from && cancel.Reason != "lifespan" {
			cm := etf.Tuple{
				etf.Atom("$saga_cancel"),
				sp.Self(),
				etf.Tuple{
					cancel.TransactionID,
					etf.Ref(tx.origin),
					cancel.Reason,
				},
			}
			sp.Send(tx.parents[0], cm)
		}
	}

	// send cancel to all next sagas except the saga this cancel came from
	sp.mutexNext.Lock()
	for nxtid, nxt := range tx.next {
		ref := etf.Ref(nxtid)
		// remove monitor from the next saga
		sp.DemonitorProcess(ref)
		delete(sp.next, nxtid)
		nxt.cancelTimer()

		if cancel.Reason == "lifespan" {
			// do not send if the cancelation caused by lifespan timer
			continue
		}
		if ref == cancel.Origin {
			// do not send to the parent if it came from there
			continue
		}

		cm := etf.Tuple{
			etf.Atom("$saga_cancel"),
			sp.Self(),
			etf.Tuple{
				cancel.TransactionID,
				ref,
				cancel.Reason,
			},
		}
		if err := sp.Send(nxt.Saga, cm); err != nil {
			errmessage := MessageSagaError{
				TransactionID: tx.id,
				NextID:        nxtid,
				Error:         "can't send cancel message",
				Details:       err.Error(),
			}
			sp.Send(sp.Self(), errmessage)
		}
	}
	sp.mutexNext.Unlock()

	// remove tx from this saga
	sp.mutexTXS.Lock()
	delete(sp.txs, tx.id)
	sp.mutexTXS.Unlock()
}

func (sp *SagaProcess) commitTX(tx *SagaTransaction, final interface{}) {
	tx.cancelTimer()
	// remove tx from this saga
	sp.mutexTXS.Lock()
	delete(sp.txs, tx.id)
	sp.mutexTXS.Unlock()

	// send commit message to all workers
	for _, pid := range tx.jobs {
		// unlink before this worker stopped
		sp.Unlink(pid)
		// do nothing if 2PC option is disabled
		if tx.options.TwoPhaseCommit == false {
			continue
		}
		// send commit message
		sp.Cast(pid, messageSagaJobCommit{final: final})
	}
	// remove monitor from parent saga
	sp.DemonitorProcess(tx.monitor)

	sp.mutexNext.Lock()
	for nxtid, nxt := range tx.next {
		ref := etf.Ref(nxtid)
		// remove monitor from the next saga
		sp.DemonitorProcess(ref)

		delete(sp.next, nxtid)
		nxt.cancelTimer()
		// send commit message
		if tx.options.TwoPhaseCommit == false {
			continue
		}
		cm := etf.Tuple{
			etf.Atom("$saga_commit"),
			sp.Self(),
			etf.Tuple{
				etf.Ref(tx.id), // tx id
				ref,            // origin (next_id)
				final,          // final result
			},
		}
		if err := sp.Send(nxt.Saga, cm); err != nil {
			errmessage := MessageSagaError{
				TransactionID: tx.id,
				NextID:        nxtid,
				Error:         "can't send commit message",
				Details:       err.Error(),
			}
			sp.Send(sp.Self(), errmessage)
		}
	}
	sp.mutexNext.Unlock()

}

func (sp *SagaProcess) handleSagaExit(exit MessageExit) error {
	sp.mutexJobs.Lock()
	job, ok := sp.jobs[exit.Pid]
	sp.mutexJobs.Unlock()
	if !ok {
		// passthrough this message to HandleSagaInfo callback
		return ErrSagaJobUnknown
	}

	if exit.Reason == "lifespan" {
		sp.Unlink(job.worker.Self())
		job.worker.Exit(exit.Reason)
	} else {
		job.cancelTimer()
	}

	// remove it from saga job list
	sp.mutexJobs.Lock()
	delete(sp.jobs, exit.Pid)
	sp.mutexJobs.Unlock()

	// check if this tx is still alive
	sp.mutexTXS.Lock()
	tx, ok := sp.txs[job.TransactionID]
	sp.mutexTXS.Unlock()
	if !ok {
		// seems it was already canceled
		return SagaStatusOK
	}

	// remove it from the tx job list
	tx.Lock()
	delete(tx.jobs, job.ID)
	tx.Unlock()

	// if this job is done, don't care about the termination reason
	if job.done {
		return SagaStatusOK
	}

	if exit.Reason != "normal" {
		return sp.behavior.HandleJobFailed(sp, job.TransactionID, job.ID, exit.Reason)
	}

	// seems no result received from this worker
	return sp.behavior.HandleJobFailed(sp, job.TransactionID, job.ID, "no result")
}

func (sp *SagaProcess) handleSagaDown(down MessageDown) error {

	sp.mutexNext.Lock()
	tx, ok := sp.next[SagaNextID(down.Ref)]
	sp.mutexNext.Unlock()
	if ok {
		// got DOWN message from the next saga
		empty := etf.Pid{}
		reason := fmt.Sprintf("next saga %s is down", down.Pid)
		if down.Pid == empty {
			// monitored by name
			reason = fmt.Sprintf("next saga %s is down", down.ProcessID)
		}
		message := etf.Tuple{
			etf.Atom("$saga_cancel"),
			down.Pid,
			etf.Tuple{etf.Ref(tx.id), down.Ref, reason},
		}
		sp.Send(sp.Self(), message)
		return nil
	}

	sp.mutexTXS.Lock()
	for _, tx := range sp.txs {
		if down.Ref != tx.monitor {
			continue
		}

		// got DOWN message from the parent saga
		reason := fmt.Sprintf("parent saga %s is down", down.Pid)
		message := etf.Tuple{
			etf.Atom("$saga_cancel"),
			down.Pid,
			etf.Tuple{etf.Ref(tx.id), down.Ref, reason},
		}
		sp.Send(sp.Self(), message)
		sp.mutexTXS.Unlock()
		return nil
	}
	sp.mutexTXS.Unlock()

	// down.Ref is unknown. Return ErrSagaUnknown to passthrough
	// this message to HandleSagaInfo callback
	return ErrSagaUnknown
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

	sp := process.State.(*SagaProcess)

	switch m := message.(type) {
	case messageSagaJobResult:
		sp.mutexJobs.Lock()
		job, ok := sp.jobs[m.pid]
		sp.mutexJobs.Unlock()
		if !ok {
			// kill this process
			if worker := process.ProcessByPid(m.pid); worker != nil {
				process.Unlink(worker.Self())
				worker.Kill()
			}
			status = SagaStatusOK
			break
		}
		job.done = true
		job.cancelTimer()

		sp.mutexTXS.Lock()
		tx, ok := sp.txs[job.TransactionID]
		sp.mutexTXS.Unlock()

		if !ok {
			// tx is already canceled. kill this worker if its still alive (tx might have had
			// 2PC enabled, and the worker is waiting for the commit message)
			process.Unlink(job.worker.Self())
			job.worker.Kill()
			status = SagaStatusOK
			break
		}

		// remove this job from the tx job list, but do not remove
		// from the sp.jobs (will be removed once worker terminated)
		if tx.options.TwoPhaseCommit == false {
			tx.Lock()
			delete(tx.jobs, job.ID)
			tx.Unlock()
		}

		status = sp.behavior.HandleJobResult(sp, job.TransactionID, job.ID, m.result)

	case messageSagaJobInterim:
		sp.mutexJobs.Lock()
		job, ok := sp.jobs[m.pid]
		sp.mutexJobs.Unlock()
		if !ok {
			// kill this process
			if worker := process.ProcessByPid(m.pid); worker != nil {
				process.Unlink(worker.Self())
				worker.Kill()
			}
			// tx was canceled. just ignore it
			status = SagaStatusOK
			break
		}
		status = sp.behavior.HandleJobInterim(sp, job.TransactionID, job.ID, m.interim)

	default:
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
	}

	if err := etf.TermIntoStruct(message, &mSaga); err != nil {
		return sp.behavior.HandleSagaInfo(sp, message)
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

//
// default Saga callbacks
//

func (gs *Saga) HandleTxInterim(process *SagaProcess, id SagaTransactionID, from SagaNextID, interim interface{}) SagaStatus {
	fmt.Printf("HandleTxInterim: [%v %v] unhandled message %#v\n", id, from, interim)
	return ServerStatusOK
}
func (gs *Saga) HandleTxCommit(process *SagaProcess, id SagaTransactionID, final interface{}) SagaStatus {
	fmt.Printf("HandleTxCommit: [%v] unhandled message\n", id)
	return ServerStatusOK
}
func (gs *Saga) HandleTxDone(process *SagaProcess, id SagaTransactionID, result interface{}) (interface{}, SagaStatus) {
	return nil, fmt.Errorf("Saga [%v:%v] has no implementaion of HandleTxDone method", process.Self(), process.Name())
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
func (gs *Saga) HandleSagaDirect(process *SagaProcess, message interface{}) (interface{}, error) {
	return nil, ErrUnsupportedRequest
}

func (gs *Saga) HandleJobResult(process *SagaProcess, id SagaTransactionID, from SagaJobID, result interface{}) SagaStatus {
	fmt.Printf("HandleJobResult: [%v %v] unhandled message %#v\n", id, from, result)
	return SagaStatusOK
}
func (gs *Saga) HandleJobInterim(process *SagaProcess, id SagaTransactionID, from SagaJobID, interim interface{}) SagaStatus {
	fmt.Printf("HandleJobInterim: [%v %v] unhandled message %#v\n", id, from, interim)
	return SagaStatusOK
}
func (gs *Saga) HandleJobFailed(process *SagaProcess, id SagaTransactionID, from SagaJobID, reason string) SagaStatus {
	fmt.Printf("HandleJobFailed: [%v %v] unhandled message. reason %q\n", id, from, reason)
	return nil
}
