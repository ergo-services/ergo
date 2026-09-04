package local

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// TestLocalEventRegisterOptions: on a live node the RegisterEvent record carries
// the options the producer passed, so a test can assert Notify without reaching
// for the producer's notification traffic.
func TestLocalEventRegisterOptions(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")

	prod := n.Spawn(factoryProducer, gen.ProcessOptions{})
	evAny, err := n.Call(prod, evtRegister{})
	check.NoError(t, err)
	event := evAny.(gen.Event)

	n.ShouldRegisterEvent().From(prod).Name(event.Name).
		Notify(true).Buffer(10).Open(false).Once().Assert()
	n.ShouldRegisterEvent().From(prod).Notify(false).None().Assert()

	// the live node agrees with the record
	info, err := n.Native().EventInfo(event)
	check.NoError(t, err)
	check.Equal(t, true, info.Notify)
	check.Equal(t, 10, info.BufferSize)
}
