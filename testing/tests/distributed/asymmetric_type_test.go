package distributed

import (
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// senderOnlyValue is meant to be registered in EDF on the sender node only.
type senderOnlyValue struct{ X int }

// TestDistSendAsymmetricType: a value whose type is registered in EDF on the
// sender node but NOT on the receiver: the sender encodes and sends it fine, but
// the receiver cannot decode it, so it is not delivered.
//
// SKIPPED: not reproducible today. EDF type registration is process-global
// (node/network.go RegisterType feeds the shared edf registry), so two in-process
// stage nodes share one registry and asymmetry cannot be set up. Enable once EDF
// registration is per-node / per-connection.
func TestDistSendAsymmetricType(t *testing.T) {
	t.Skip("TODO: enable when EDF type registration is per-node/per-connection (currently global)")

	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")

	// register the type on the sender only (requires per-node registration)
	check.NoError(t, n1.Native().Network().RegisterType(senderOnlyValue{}))
	s.Connect(n1, n2)

	snd := n1.Spawn(factorySender, gen.ProcessOptions{})
	p := n2.Spawn(factoryPong, gen.ProcessOptions{})

	mk := n2.Mark()
	// the sender encodes and sends successfully (type known on n1)...
	res, err := n1.Call(snd, sendCmd{Kind: "send", To: p, Msg: senderOnlyValue{X: 1}})
	check.NoError(t, err)
	check.Equal(t, "", res)

	// barrier: a following good message arrives; FIFO guarantees the undecodable
	// one was already processed (and dropped) by the receiver by then
	_, err = n1.Call(snd, sendCmd{Kind: "send", To: p, Msg: "barrier"})
	check.NoError(t, err)
	n2.ShouldDeliver().To(p).Message("barrier").Since(mk).Once().Within(time.Second).Must()

	// ...but the undecodable value was never delivered
	n2.ShouldDeliver().To(p).Where(func(d check.Delivered) bool {
		_, ok := d.Message.(senderOnlyValue)
		return ok
	}).Since(mk).None().Assert()
}
