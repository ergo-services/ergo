package check

import "ergo.services/ergo/lib"

// Recorder is the per-harness sink of observed Records and a check.Source.
//
// Producers (actor loops, the routing core, connections, or a mock process) push
// concurrently via Put; the queue is multi-producer/single-consumer. The consumer
// side (Records, drained by Mark and the assertion engine) is single-goroutine: a
// harness's Mark / Should* / assertions must all be called from one goroutine (the
// test goroutine). Test-level concurrency that only drives the harness API is fine;
// concurrent assertions on the same recorder are not supported.
type Recorder struct {
	q      lib.QueueMPSC
	stored []Record
}

// NewRecorder creates an empty recorder.
func NewRecorder() *Recorder {
	return &Recorder{q: lib.NewQueueMPSC()}
}

// Put appends a record (called by harness decorators / mocks).
func (r *Recorder) Put(rec Record) { r.q.Push(rec) }

// Records drains the queue into the append-only history and returns it. The slice
// is shared (no copy): the history only grows, callers only read it, and the
// single-consumer contract means no concurrent mutation.
func (r *Recorder) Records() []Record {
	for {
		v, ok := r.q.Pop()
		if ok == false {
			break
		}
		r.stored = append(r.stored, v.(Record))
	}
	return r.stored
}

// Mark returns the current recorder position. Pass it to an assertion's Since(mark)
// to scope matching to records observed after this point.
func (r *Recorder) Mark() int { return len(r.Records()) }
