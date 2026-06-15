// Package mock provides standalone, dumb mocks of the gen.* interfaces for unit
// testing code that consumes them. Each mock implements its interface in full, with
// an On<Method> override for every method; an unset method returns a safe default
// (zero value, a synthetic identifier, or success). The mocks are independent of the
// testing/unit harness: use them to inject a controllable gen.Node, gen.Process,
// gen.MetaProcess, gen.Log, gen.Cron, gen.Network or gen.RemoteNode into the code
// under test.
//
// Each type has two constructors:
//
//   - NewX()     a dumb mock: overrides plus safe defaults, no recording.
//   - NewXT(t)   the same, plus a check.Recorder for egress and the embedded
//     check.Asserter, so the check Should* grammar is available
//     (mock.NewNodeT(t).ShouldSend()...). A node minted with NewNodeT
//     hands its sub-mocks (Log/Network/Cron) the same recorder, so every
//     record collates into one ordered stream.
//
// The mocks never fail the test on an unconfigured call (that loud-failure behavior
// belongs to the testing/unit harness); override a method when a non-default result
// is needed.
package mock

import (
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// recorder is embedded by every mock: the optional record/assert sink. A dumb mock
// (NewX) holds the zero value (rec == nil) and records nothing; a NewXT mock records
// egress into rec and asserts through the embedded Asserter. It holds only pointers
// and an interface, so it is safe to copy by value when a parent shares it with a
// sub-mock.
type recorder struct {
	*check.Asserter
	rec *check.Recorder
	t   check.T
}

func newRecorder(t check.T) recorder {
	rec := check.NewRecorder()
	return recorder{Asserter: check.NewAsserter(t, rec), rec: rec, t: t}
}

// put records r when recording is enabled (NewXT); a dumb mock drops it.
func (m recorder) put(r check.Record) {
	if m.rec != nil {
		m.rec.Put(r)
	}
}

// the default identity used by a standalone mock for the From of its records and
// for synthesized PIDs/aliases/refs.
const mockNode = gen.Atom("mock@localhost")

func synthPID(id uint64) gen.PID { return gen.PID{Node: mockNode, ID: id, Creation: 1} }
func synthAlias(id uint64) gen.Alias {
	return gen.Alias{Node: mockNode, Creation: 1, ID: [3]uint64{id, 0, 0}}
}
func synthRef(id uint64) gen.Ref {
	return gen.Ref{Node: mockNode, Creation: 1, ID: [3]uint64{id, 0, 0}}
}
