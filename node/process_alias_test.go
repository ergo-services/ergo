package node

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

// deleting a non-head alias must keep the other aliases and drop only the target.
func TestProcessDeleteAliasKeepsOthers(t *testing.T) {
	n := &node{creation: 1, log: createLog(gen.LogLevelDisabled, nil)}
	p := &process{
		pid:  gen.PID{Node: "n@localhost", ID: 100, Creation: 1},
		node: n,
		core: mock.NewCoreT(t),
	}
	p.state = int32(gen.ProcessStateRunning)

	a1, err := p.CreateAlias()
	check.NoError(t, err)
	a2, err := p.CreateAlias()
	check.NoError(t, err)

	// a2 is not the head of the list
	check.NoError(t, p.DeleteAlias(a2))

	got := p.Aliases()
	check.Equal(t, 1, len(got))
	check.Equal(t, a1, got[0])

	// the deleted alias is unregistered from the node, the survivor is not
	_, hasA1 := n.aliases.Load(a1)
	check.True(t, hasA1)
	_, hasA2 := n.aliases.Load(a2)
	check.False(t, hasA2)
}
