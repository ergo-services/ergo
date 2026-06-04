package distributed

// Covers ack-matching for cross-node SendResponseImportant /
// SendResponseErrorImportant. The bug previously exercised by this
// suite: the responder's waitResponse expects a ref carrying the
// remote caller's node prefix (since protoRequestPID reconstructs
// message.Ref with c.peer), while the peer's auto-ack arrives at the
// responder's side reconstructed with c.core.Name(), producing a
// guaranteed ref mismatch and a 5s timeout on every Important reply.
//
// The caller's Call returns immediately (the response is sent via
// RouteSendResponse before waitResponse), so the bug is invisible to
// the caller. To catch it, each responder reports the return value of
// SendResponseImportant back to the driver via a side-channel Send.
// With the bug present, the side-channel arrives carrying ErrTimeout
// after ~5 seconds; with the fix it arrives immediately with nil.
//
// Three scenarios:
//   1. Direct responder: HandleCall fires SendResponseImportant and
//      returns (nil, nil) to defer the reply.
//   2. Direct error responder: SendResponseErrorImportant variant.
//   3. Deferred-reply across three nodes: p1.Call(p2) -> p2 wraps
//      (From, Ref, Msg) into a struct and Sends to p3 on a third
//      node -> p3.SendResponseImportant(p1, wrappedRef, resp). The
//      ref originally generated on node1 is preserved through EDF
//      encoding inside the wrapper, never reconstructed on the
//      request path at node3.

import (
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/net/edf"
)

// -----------------------------------------------------------------------------
// Wire types
// -----------------------------------------------------------------------------

// t12Wrapped is the deferred-reply envelope passed from p2 to p3 over
// the cluster. EDF preserves the embedded gen.Ref's Node atom intact
// (cross-node atom cache), so p3 sees the ref exactly as it was at
// p2's HandleCall, which is exactly as p1 originally generated it.
type t12Wrapped struct {
	From gen.PID
	Ref  gen.Ref
	Msg  int
}

// t12Status carries the return value of SendResponseImportant /
// SendResponseErrorImportant back to the test driver. ErrText is
// empty on success, non-empty on failure (e.g. "timeout").
type t12Status struct {
	ErrText string
}

func init() {
	for _, t := range []any{t12Wrapped{}, t12Status{}} {
		if err := edf.RegisterTypeOf(t); err != nil && err != gen.ErrTaken {
			panic(err)
		}
	}
}

func t12statusText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// -----------------------------------------------------------------------------
// Direct responders
// -----------------------------------------------------------------------------

// t12pong handles a Call by replying via SendResponseImportant from
// inside HandleCall, then reports the result back to the driver PID
// (received via spawn args) and returns (nil, nil) to defer the reply.
type t12pong struct {
	act.Actor
	reporter gen.PID
}

func factory_t12pong() gen.ProcessBehavior { return &t12pong{} }

func (p *t12pong) Init(args ...any) error {
	if len(args) == 0 {
		return fmt.Errorf("t12pong: missing reporter PID arg")
	}
	r, ok := args[0].(gen.PID)
	if ok == false {
		return fmt.Errorf("t12pong: invalid args[0] type %T", args[0])
	}
	p.reporter = r
	return nil
}

func (p *t12pong) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	err := p.SendResponseImportant(from, ref, request)
	if sendErr := p.Send(p.reporter, t12Status{ErrText: t12statusText(err)}); sendErr != nil {
		return nil, sendErr
	}
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// t12errPong handles a Call by replying via SendResponseErrorImportant.
type t12errPong struct {
	act.Actor
	reporter gen.PID
}

func factory_t12errPong() gen.ProcessBehavior { return &t12errPong{} }

var t12errPongResponse = fmt.Errorf("t12: responded with error")

func (p *t12errPong) Init(args ...any) error {
	if len(args) == 0 {
		return fmt.Errorf("t12errPong: missing reporter PID arg")
	}
	r, ok := args[0].(gen.PID)
	if ok == false {
		return fmt.Errorf("t12errPong: invalid args[0] type %T", args[0])
	}
	p.reporter = r
	return nil
}

func (p *t12errPong) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	err := p.SendResponseErrorImportant(from, ref, t12errPongResponse)
	if sendErr := p.Send(p.reporter, t12Status{ErrText: t12statusText(err)}); sendErr != nil {
		return nil, sendErr
	}
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// -----------------------------------------------------------------------------
// Deferred-reply pair: forwarder on node2 + final responder on node3
// -----------------------------------------------------------------------------

// t12forwarder receives a Call, captures (From, Ref, request) into
// t12Wrapped, and Sends it to the final-responder PID (configured via
// spawn args). Returns (nil, nil) to defer the reply, which the final
// responder will produce via SendResponseImportant directly to the
// original caller.
type t12forwarder struct {
	act.Actor
	to gen.PID
}

func factory_t12forwarder() gen.ProcessBehavior { return &t12forwarder{} }

func (f *t12forwarder) Init(args ...any) error {
	if len(args) == 0 {
		return fmt.Errorf("t12forwarder: missing final-responder PID arg")
	}
	to, ok := args[0].(gen.PID)
	if ok == false {
		return fmt.Errorf("t12forwarder: invalid args[0] type %T", args[0])
	}
	f.to = to
	return nil
}

func (f *t12forwarder) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	v, ok := request.(int)
	if ok == false {
		return nil, fmt.Errorf("t12forwarder: unexpected request type %T", request)
	}
	if err := f.Send(f.to, t12Wrapped{From: from, Ref: ref, Msg: v}); err != nil {
		return nil, err
	}
	return nil, nil
}

// t12finalResponder receives a t12Wrapped and replies via
// SendResponseImportant directly to the original caller (carried in
// the wrapped From + Ref), then reports the result back to the
// reporter PID configured via spawn args.
type t12finalResponder struct {
	act.Actor
	reporter gen.PID
}

func factory_t12finalResponder() gen.ProcessBehavior { return &t12finalResponder{} }

func (r *t12finalResponder) Init(args ...any) error {
	if len(args) == 0 {
		return fmt.Errorf("t12finalResponder: missing reporter PID arg")
	}
	rep, ok := args[0].(gen.PID)
	if ok == false {
		return fmt.Errorf("t12finalResponder: invalid args[0] type %T", args[0])
	}
	r.reporter = rep
	return nil
}

func (r *t12finalResponder) HandleMessage(from gen.PID, message any) error {
	w, ok := message.(t12Wrapped)
	if ok == false {
		return nil
	}
	err := r.SendResponseImportant(w.From, w.Ref, w.Msg)
	if sendErr := r.Send(r.reporter, t12Status{ErrText: t12statusText(err)}); sendErr != nil {
		return sendErr
	}
	return err
}

// -----------------------------------------------------------------------------
// Test driver actor (lives on node1, runs the cases)
// -----------------------------------------------------------------------------

// t12 is a stateful driver: a case's HandleMessage entry kicks off
// the synchronous Call and validates the response, then leaves the
// case "open" awaiting the t12Status side-channel arrival. The next
// HandleMessage matches the status and signals testcase.err.
type t12 struct {
	act.Actor

	node2 gen.Atom
	node3 gen.Atom

	testcase     *testcase
	pongExpected int  // value the Call should have returned (validated on status)
	pongGot      int  // what the Call actually returned
	pongMismatch bool // set true if pong didn't match
	pongFailed   bool // set true if Call returned an error
}

func factory_t12() gen.ProcessBehavior { return &t12{} }

func (t *t12) Init(args ...any) error {
	t.node2 = args[0].(gen.Atom)
	if len(args) > 1 {
		t.node3 = args[1].(gen.Atom)
	}
	return nil
}

func (t *t12) HandleMessage(from gen.PID, message any) error {
	// Side-channel: a responder reporting SendResponseImportant result.
	if status, ok := message.(t12Status); ok {
		t.handleStatus(status)
		return nil
	}

	if t.testcase == nil {
		t.testcase = message.(*testcase)
		message = initcase{}
	}
	method := reflect.ValueOf(t).MethodByName(t.testcase.name)
	if method.IsValid() == false {
		t.testcase.err <- fmt.Errorf("unknown method %q", t.testcase.name)
		t.testcase = nil
		return nil
	}
	method.Call([]reflect.Value{reflect.ValueOf(message)})
	return nil
}

// handleStatus consumes the responder's side-channel report and
// signals testcase.err based on the combined state (Call result +
// SendResponseImportant return value).
func (t *t12) handleStatus(status t12Status) {
	defer func() {
		t.testcase = nil
		t.pongExpected = 0
		t.pongGot = 0
		t.pongMismatch = false
		t.pongFailed = false
	}()
	if t.testcase == nil {
		return
	}
	if t.pongFailed {
		t.testcase.err <- fmt.Errorf("Call failed (status err %q)", status.ErrText)
		return
	}
	if t.pongMismatch {
		t.testcase.err <- fmt.Errorf("pong mismatch: got %d want %d (status err %q)",
			t.pongGot, t.pongExpected, status.ErrText)
		return
	}
	if status.ErrText != "" {
		t.testcase.err <- fmt.Errorf("SendResponseImportant returned non-nil: %s", status.ErrText)
		return
	}
	t.testcase.err <- nil
}

// TestSendResponseImportantRemote: scenario 1 - direct responder using
// SendResponseImportant inside HandleCall. The Call must succeed AND
// the responder's SendResponseImportant must return nil (verified via
// t12Status side-channel).
func (t *t12) TestSendResponseImportantRemote(input any) {
	pid, err := t.RemoteSpawn(t.node2, "t12pong", gen.ProcessOptions{}, t.PID())
	if err != nil {
		t.testcase.err <- err
		t.testcase = nil
		return
	}

	pingvalue := 123
	t.pongExpected = pingvalue
	pong, err := t.Call(pid, pingvalue)
	if err != nil {
		t.pongFailed = true
		return // wait for status side-channel (may arrive with timeout error)
	}
	got, ok := pong.(int)
	if ok == false || got != pingvalue {
		t.pongMismatch = true
		if ok {
			t.pongGot = got
		}
		return
	}
	t.pongGot = got
	// Wait for t12Status to validate SendResponseImportant returned nil.
}

// TestSendResponseErrorImportantRemote: scenario 2 - direct error
// responder. Caller's Call returns the responded error, AND the
// responder's SendResponseErrorImportant must return nil.
func (t *t12) TestSendResponseErrorImportantRemote(input any) {
	pid, err := t.RemoteSpawn(t.node2, "t12errPong", gen.ProcessOptions{}, t.PID())
	if err != nil {
		t.testcase.err <- err
		t.testcase = nil
		return
	}

	pingvalue := 123
	t.pongExpected = pingvalue
	_, err = t.Call(pid, pingvalue)
	if err == nil {
		t.pongFailed = true
		return
	}
	if err.Error() != t12errPongResponse.Error() {
		t.pongMismatch = true
		return
	}
	t.pongGot = pingvalue
	// Wait for t12Status confirming SendResponseErrorImportant returned nil.
}

// TestSendResponseImportantDeferredReply: scenario 3 - the most
// demanding cross-node ack path. The Ref originates on node1, is
// preserved unchanged inside t12Wrapped through node2 to node3 (EDF
// keeps the embedded Node atom intact), and the responder on node3
// uses SendResponseImportant against that ref. The ack-waiter at
// node3 must match an ack reconstructed locally at node3 from wire
// IDs only.
func (t *t12) TestSendResponseImportantDeferredReply(input any) {
	finalPID, err := t.RemoteSpawn(t.node3, "t12finalResponder", gen.ProcessOptions{}, t.PID())
	if err != nil {
		t.testcase.err <- err
		t.testcase = nil
		return
	}
	forwarderPID, err := t.RemoteSpawn(t.node2, "t12forwarder", gen.ProcessOptions{}, finalPID)
	if err != nil {
		t.testcase.err <- err
		t.testcase = nil
		return
	}

	pingvalue := 456
	t.pongExpected = pingvalue
	pong, err := t.Call(forwarderPID, pingvalue)
	if err != nil {
		t.pongFailed = true
		return
	}
	got, ok := pong.(int)
	if ok == false || got != pingvalue {
		t.pongMismatch = true
		if ok {
			t.pongGot = got
		}
		return
	}
	t.pongGot = got
	// Wait for t12Status confirming SendResponseImportant returned nil.
}

// -----------------------------------------------------------------------------
// Top-level
// -----------------------------------------------------------------------------

func TestT12SendResponseImportantRemote(t *testing.T) {
	startNode := func(name string) gen.Node {
		opts := gen.NodeOptions{}
		opts.Network.Cookie = "t12-cookie"
		opts.Log.DefaultLogger.Disable = true
		opts.Log.Level = gen.LogLevelTrace
		opts.Security.ExposeEnvRemoteSpawn = true
		node, err := ergo.StartNode(gen.Atom(name+"@localhost"), opts)
		if err != nil {
			t.Fatalf("start node %s: %s", name, err)
		}
		return node
	}

	node1 := startNode("distT12node1")
	defer node1.StopForce()
	node2 := startNode("distT12node2")
	defer node2.StopForce()
	node3 := startNode("distT12node3")
	defer node3.StopForce()

	if err := node2.Network().EnableSpawn("t12pong", factory_t12pong); err != nil {
		t.Fatal(err)
	}
	if err := node2.Network().EnableSpawn("t12errPong", factory_t12errPong); err != nil {
		t.Fatal(err)
	}
	if err := node2.Network().EnableSpawn("t12forwarder", factory_t12forwarder); err != nil {
		t.Fatal(err)
	}
	if err := node3.Network().EnableSpawn("t12finalResponder", factory_t12finalResponder); err != nil {
		t.Fatal(err)
	}

	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}
	if _, err := node1.Network().GetNode(node3.Name()); err != nil {
		t.Fatal(err)
	}
	if _, err := node2.Network().GetNode(node3.Name()); err != nil {
		t.Fatal(err)
	}

	pid, err := node1.Spawn(factory_t12, gen.ProcessOptions{}, node2.Name(), node3.Name())
	if err != nil {
		t.Fatal(err)
	}

	cases := []*testcase{
		{"TestSendResponseImportantRemote", nil, nil, make(chan error)},
		{"TestSendResponseErrorImportantRemote", nil, nil, make(chan error)},
		{"TestSendResponseImportantDeferredReply", nil, nil, make(chan error)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node1.Send(pid, tc)
			// 2s budget: with the fix, status arrives in <1ms; with
			// the bug, the responder spends DefaultRequestTimeout
			// (5s) inside waitResponse before reporting, so the
			// driver never signals testcase.err in time and we see
			// gen.ErrTimeout here.
			if err := tc.wait(2); err != nil {
				t.Fatal(err)
			}
		})
	}
}
