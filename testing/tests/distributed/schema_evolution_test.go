package distributed

import (
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// evoSmokeMsg is a registered struct sent over a connection that negotiated
// EnableSchemaEvolution, exercising the length-prefixed struct encode/decode path
// across a real connection. The field-count drift itself (skip extra / zero-fill
// missing) is covered by the edf unit tests; in-process nodes share one edf
// registry, so two layouts of one type cannot be set up here.
type evoSmokeMsg struct {
	N    int64
	Name string
}

// TestDistSchemaEvolutionRoundTrip: with EnableSchemaEvolution negotiated on both
// nodes, a registered struct survives a real cross-node round-trip intact. Covers
// the flag negotiation (NodeFlags && PeerFlags -> edf Options.SchemaEvolution) and
// the body-length-prefix wire path end to end.
func TestDistSchemaEvolutionRoundTrip(t *testing.T) {
	flags := gen.DefaultNetworkFlags
	flags.EnableSchemaEvolution = true

	s := stage.New(t)
	n1 := s.StartNode("n1", stage.NodeOptions{NetworkFlags: flags})
	n2 := s.StartNode("n2", stage.NodeOptions{NetworkFlags: flags})
	check.NoError(t, n1.Native().Network().RegisterType(evoSmokeMsg{}))
	check.NoError(t, n2.Native().Network().RegisterType(evoSmokeMsg{}))
	s.Connect(n1, n2)

	snd := n1.Spawn(factorySender, gen.ProcessOptions{})
	p := n2.Spawn(factoryPong, gen.ProcessOptions{})

	want := evoSmokeMsg{N: 42, Name: "hello"}
	mk := n2.Mark()
	res, err := n1.Call(snd, sendCmd{Kind: "send", To: p, Msg: want})
	check.NoError(t, err)
	check.Equal(t, "", res)

	n2.ShouldDeliver().To(p).Where(func(d check.Delivered) bool {
		m, ok := d.Message.(evoSmokeMsg)
		return ok && m == want
	}).Since(mk).Once().Within(time.Second).Must()
}
