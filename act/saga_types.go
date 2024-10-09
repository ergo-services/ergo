package act

import (
	"ergo.services/ergo/gen"
	"fmt"
)

// SagaTransactionOptions
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

// SagaOptions
type SagaOptions struct {
	// MaxTransactions defines the limit for the number of active transactions. Default: 0 (unlimited)
	MaxTransactions uint
	// Worker
	Worker SagaWorkerBehavior
}

// SagaTransactionID
type SagaTransactionID gen.Ref

// String
func (id SagaTransactionID) String() string {
	return fmt.Sprintf("SagaTX#%d.%d.%d", id.ID[0], id.ID[1], id.ID[2])
}

// SagaTransaction
type SagaTransaction struct {
	id      SagaTransactionID
	options SagaTransactionOptions
	origin  SagaNextID               // next id on a saga it came from
	monitor gen.Ref                  // monitor parent saga
	next    map[SagaNextID]*SagaNext // where were sent
	jobs    map[SagaJobID]gen.PID
	arrival int64     // when it arrived on this saga
	parents []gen.PID // sagas trace

	done bool // do not allow send result more than once if 2PC is set
	// cancelTimer CancelFunc
}

// SagaJob
type SagaJob struct {
	ID            SagaJobID
	TransactionID SagaTransactionID
	Value         interface{}

	// internal
	options SagaJobOptions
	saga    gen.PID
	commit  bool
	// worker      Process
	done bool
	// cancelTimer CancelFunc
}

// SagaJobOptions
type SagaJobOptions struct {
	Timeout uint
}

// SagaNextID
type SagaNextID gen.Ref

// String
func (id SagaNextID) String() string {
	return fmt.Sprintf("SagaNext#%d.%d.%d", id.ID[0], id.ID[1], id.ID[2])
}

// SagaNext
type SagaNext struct {
	// Saga etf.Pid, string (for the locally registered process), gen.ProcessID{process, node} (for the remote process)
	Saga any
	// Value a value for the invoking HandleTxNew on a next hop.
	Value any
	// Timeout how long this Saga will be waiting for the result from the next hop. Default - 10 seconds
	Timeout uint
	// TrapCancel if the next saga fails, it will transform the cancel signal into the regular message gen.MessageSagaCancel, and HandleSagaInfo callback will be invoked.
	TrapCancel bool

	// internal
	done bool // for 2PC case
	// cancelTimer CancelFunc
}

// SagaJobID
type SagaJobID gen.Ref

// String
func (id SagaJobID) String() string {
	return fmt.Sprintf("Job#%d.%d.%d", id.ID[0], id.ID[1], id.ID[2])
}
