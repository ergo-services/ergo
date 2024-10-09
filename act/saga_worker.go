package act

import "ergo.services/ergo/gen"

// SagaWorkerBehavior
type SagaWorkerBehavior interface {
	gen.ProcessBehavior
	// Mandatory callbacks

	// HandleJobStart invoked on a worker start
	HandleJobStart(job SagaJob) error
	// HandleJobCancel invoked if transaction was canceled before the termination.
	HandleJobCancel(reason error)

	// Optional callbacks

	// HandleJobCommit invoked if this job was a part of the transaction
	// with enabled TwoPhaseCommit option. All workers involved in this TX
	// handling are receiving this call. Callback invoked before the termination.
	HandleJobCommit(final any)

	// HandleMessage invoked if Actor received a message sent with gen.Process.Send(...).
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
