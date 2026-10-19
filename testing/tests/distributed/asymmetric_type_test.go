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
func TestDistSendAsymmetricType(t *testing.T) {
	s := stage.New(t)
	n1 := s.StartNode("n1")
	n2 := s.StartNode("n2")

	// register the type on the sender only
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

// lateValue is registered on both nodes after they are connected.
type lateValue struct{ X int }

// TestDistSendTypeRegisteredAfterConnect: types registered on both nodes of an
// established connection are delivered without reconnecting.
func TestDistSendTypeRegisteredAfterConnect(t *testing.T) {
	s := stage.New(t)
	n1 := s.StartNode("n1")
	n2 := s.StartNode("n2")
	s.Connect(n1, n2)

	check.NoError(t, n1.Native().Network().RegisterType(lateValue{}))
	check.NoError(t, n2.Native().Network().RegisterType(lateValue{}))

	snd := n1.Spawn(factorySender, gen.ProcessOptions{})
	p := n2.Spawn(factoryPong, gen.ProcessOptions{})

	mk := n2.Mark()
	res, err := n1.Call(snd, sendCmd{Kind: "send", To: p, Msg: lateValue{X: 7}})
	check.NoError(t, err)
	check.Equal(t, "", res)
	n2.ShouldDeliver().To(p).Message(lateValue{X: 7}).Since(mk).Once().Within(time.Second).Must()
}
