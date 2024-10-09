package act

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// SagaBehavior interface
type SagaBehavior interface {
	gen.ProcessBehavior

	//
	// Mandatory callbacks
	//

	Init(args ...any) (SagaOptions, error)

	// HandleTxNew invokes on a new TX receiving by this saga.
	HandleTxNew(id SagaTransactionID, value any) error

	// HandleTxResult invoked on a receiving result from the next saga
	HandleTxResult(id SagaTransactionID, from SagaNextID, result any) error

	// HandleTxCancel invoked on a request of transaction cancelation.
	HandleTxCancel(id SagaTransactionID, reason error) error

	//
	// Optional callbacks
	//

	// HandleTxDone invoked when the transaction is done on a saga where it was created.
	// It returns the final result and SagaStatus. The commit message will deliver the final
	// result to all participants of this transaction (if it has enabled the TwoPhaseCommit option).
	// Otherwise the final result will be ignored.
	HandleTxDone(id SagaTransactionID, result any) (any, error)

	// HandleTxInterim invoked if received interim result from the next hop
	HandleTxInterim(id SagaTransactionID, from SagaNextID, interim any) error

	// HandleTxCommit invoked if TwoPhaseCommit option is enabled for the given TX.
	// All sagas involved in this TX receive a commit message with final value and invoke this callback.
	// The final result has a value returned by HandleTxDone on a Saga created this TX.
	HandleTxCommit(id SagaTransactionID, final any) error

	//
	// Callbacks to handle result/interim from the worker(s)
	//

	// HandleJobResult
	HandleJobResult(id SagaTransactionID, from SagaJobID, result interface{}) error
	// HandleJobInterim
	HandleJobInterim(id SagaTransactionID, from SagaJobID, interim interface{}) error
	// HandleJobFailed
	HandleJobFailed(id SagaTransactionID, from SagaJobID, reason string) error

	// HandleMessage invoked if Saga received a message sent with gen.Process.Send(...).
	// Non-nil value of the returning error will cause termination of this process.
	// To stop this process normally, return gen.TerminateReasonNormal
	// or any other for abnormal termination.
	HandleMessage(from gen.PID, message any) error

	// HandleCall invoked if Actor got a synchronous request made with gen.Process.Call(...).
	// Return nil as a result to handle this request asynchronously and
	// to provide the result later using the gen.Process.SendResponse(...) method.
	HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)

	// Terminate invoked on a termination process
	Terminate(reason error)

	// HandleEvent invoked on an event message if this process got subscribed on
	// this event using gen.Process.LinkEvent or gen.Process.MonitorEvent
	HandleEvent(message gen.MessageEvent) error

	// HandleInspect invoked on the request made with gen.Process.Inspect(...)
	HandleInspect(from gen.PID, item ...string) map[string]string
}

type Saga struct {
	gen.Process

	behavior SagaBehavior
	mailbox  gen.ProcessMailbox

	options SagaOptions

	// running transactions
	txs map[SagaTransactionID]*SagaTransaction
	// next sagas where txs were sent
	next map[SagaNextID]*SagaTransaction
	// running jobs
	jobs map[gen.PID]*SagaJob
}

//
// ProcessBehavior implementation
//

// ProcessInit
func (s *Saga) ProcessInit(process gen.Process, args ...any) (rr error) {
	var ok bool

	if s.behavior, ok = process.Behavior().(SagaBehavior); ok == false {
		unknown := strings.TrimPrefix(reflect.TypeOf(process.Behavior()).String(), "*")
		return fmt.Errorf("ProcessInit: not a SagaBehavior %s", unknown)
	}

	if lib.Recover() {
		defer func() {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)
				s.Log().Panic("Saga initialization failed. Panic reason: %#v at %s[%s:%d]",
					r, runtime.FuncForPC(pc).Name(), fn, line)
				rr = gen.TerminateReasonPanic
			}
		}()
	}

	s.Process = process
	s.mailbox = process.Mailbox()

	options, err := s.behavior.Init(args...)
	if err != nil {
		return err
	}

	return nil
}
