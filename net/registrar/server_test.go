package registrar

import (
	"net"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

// newTestServer builds a server with just its in-memory registry, no listeners,
// so the register/resolve logic can be exercised without any sockets.
func newTestServer() *server {
	return &server{
		log:        mock.NewLog(),
		routes:     make(map[gen.Atom][]gen.Route),
		registered: make(map[net.Conn]gen.Atom),
	}
}

func TestServerRegisterAndResolve(t *testing.T) {
	s := newTestServer()
	check.NoError(t, s.registerNode("node1@localhost", []gen.Route{{Host: "h", Port: 1234}}, nil))

	got, err := s.resolve("node1@localhost", true)
	check.NoError(t, err)
	check.Equal(t, 1, len(got))
	check.Equal(t, uint16(1234), got[0].Port)
}

func TestServerRegisterDuplicateRejected(t *testing.T) {
	s := newTestServer()
	check.NoError(t, s.registerNode("n@localhost", []gen.Route{{Port: 1}}, nil))
	check.ErrorIs(t, s.registerNode("n@localhost", []gen.Route{{Port: 2}}, nil), gen.ErrTaken)
}

func TestServerRegisterEmptyRoutesRejected(t *testing.T) {
	s := newTestServer()
	check.ErrorIs(t, s.registerNode("n@localhost", nil, nil), gen.ErrIncorrect)
}

func TestServerResolveUnknown(t *testing.T) {
	s := newTestServer()
	_, err := s.resolve("nobody@localhost", false)
	check.ErrorIs(t, err, gen.ErrUnknown)
}

func TestServerUnregister(t *testing.T) {
	s := newTestServer()
	check.NoError(t, s.registerNode("n@localhost", []gen.Route{{Port: 1}}, nil))

	s.unregisterNode("n@localhost", nil)

	_, err := s.resolve("n@localhost", false)
	check.ErrorIs(t, err, gen.ErrUnknown)
}

// re-registering a name after it was unregistered succeeds.
func TestServerReRegisterAfterUnregister(t *testing.T) {
	s := newTestServer()
	check.NoError(t, s.registerNode("n@localhost", []gen.Route{{Port: 1}}, nil))
	s.unregisterNode("n@localhost", nil)
	check.NoError(t, s.registerNode("n@localhost", []gen.Route{{Port: 2}}, nil))
}

// resolve(docopy=true) returns a copy that does not alias the stored routes.
func TestServerResolveCopyIsolatesStorage(t *testing.T) {
	s := newTestServer()
	check.NoError(t, s.registerNode("n@localhost", []gen.Route{{Port: 1}}, nil))

	got, _ := s.resolve("n@localhost", true)
	got[0].Port = 999 // mutate the returned copy

	again, _ := s.resolve("n@localhost", true)
	check.Equal(t, uint16(1), again[0].Port) // storage untouched
}
