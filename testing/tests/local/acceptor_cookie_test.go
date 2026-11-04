package local

import (
	"testing"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/gen"
)

// TestLocalAcceptorOwnCookie: an acceptor with a cookie of its own takes the peers
// knowing that cookie and refuses the ones knowing the node cookie only.
func TestLocalAcceptorOwnCookie(t *testing.T) {
	port := freePort(t)
	opts := gen.NodeOptions{
		Network: gen.NetworkOptions{
			Cookie: "internal",
			Acceptors: []gen.AcceptorOptions{
				{Host: "127.0.0.1", Port: port, PortRange: 1, Cookie: "partner"},
			},
		},
	}
	opts.Log.DefaultLogger.Disable = true
	a, err := ergo.StartNode("acookie@localhost", opts)
	if err != nil {
		t.Fatalf("StartNode: %v", err)
	}
	defer a.StopForce()

	dialer := gen.NodeOptions{}
	dialer.Network.Cookie = "internal"
	dialer.Network.Acceptors = []gen.AcceptorOptions{{Host: "127.0.0.1", Port: freePort(t), PortRange: 1}}
	dialer.Log.DefaultLogger.Disable = true
	b, err := ergo.StartNode("bcookie@localhost", dialer)
	if err != nil {
		t.Fatalf("StartNode: %v", err)
	}
	defer b.StopForce()

	route := func(cookie string) gen.NetworkRoute {
		return gen.NetworkRoute{
			Route:            gen.Route{Host: "127.0.0.1", Port: port, TLS: false},
			Cookie:           cookie,
			HandshakeTimeout: 2 * time.Second,
		}
	}
	if _, err := b.Network().GetNodeWithRoute(a.Name(), route("internal")); err == nil {
		t.Fatal("the acceptor with its own cookie took the node cookie")
	}
	if _, err := b.Network().GetNodeWithRoute(a.Name(), route("partner")); err != nil {
		t.Fatalf("the acceptor refused its own cookie: %v", err)
	}
}
