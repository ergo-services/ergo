package proto

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

func TestConnectionInfo(t *testing.T) {
	c := &connection{
		peer:                "peer@localhost",
		peer_creation:       testPeerCreation,
		peer_version:        gen.Version{Name: "test", Release: "1"},
		peer_flags:          gen.NetworkFlags{Enable: true},
		peer_maxmessagesize: 4096,
		pool_size:           2,
		tls:                 true,
	}

	info := c.Info()
	check.Equal(t, gen.Atom("peer@localhost"), info.Node)
	check.Equal(t, c.peer_version, info.Version)
	check.Equal(t, gen.NetworkFlags{Enable: true}, info.NetworkFlags)
	check.Equal(t, 2, info.PoolSize)
	check.Equal(t, 4096, info.MaxMessageSize)
	check.True(t, info.TLS)
}

func TestMakeRequestRef(t *testing.T) {
	want := gen.Ref{Node: "me@localhost", Creation: 1, ID: [3]uint64{5, 0, 0}}
	var gotDeadline int64
	core := mock.NewCore()
	core.OnMakeRefWithDeadline(func(deadline int64) (gen.Ref, error) {
		gotDeadline = deadline
		return want, nil
	})
	c := &connection{core: core}

	check.Equal(t, want, c.makeRequestRef())
	check.True(t, gotDeadline > 0) // a deadline was attached
}
