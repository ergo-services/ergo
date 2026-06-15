package handshake

import (
	"net"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// hsNode is a minimal gen.NodeHandshake: the handshake only reads Name, Creation
// and Version from it.
type hsNode struct {
	name     gen.Atom
	creation int64
}

func (n *hsNode) Name() gen.Atom       { return n.name }
func (n *hsNode) Creation() int64      { return n.creation }
func (n *hsNode) Version() gen.Version { return gen.Version{Name: "test", Release: "1"} }

// the dialer (Start) and acceptor (Accept) negotiate over a pipe and each end up
// with the other's identity, flags and message-size limit, agreeing on one id.
func TestHandshakeRoundTrip(t *testing.T) {
	hs := Create(Options{})
	dc, ac := net.Pipe()
	t.Cleanup(func() { dc.Close(); ac.Close() })

	dialer := &hsNode{name: "dialer@localhost", creation: 100}
	acceptor := &hsNode{name: "acceptor@localhost", creation: 200}
	dialerOpts := gen.HandshakeOptions{Cookie: "secret", Flags: gen.NetworkFlags{Enable: true, EnableRemoteSpawn: true}, MaxMessageSize: 1000}
	acceptorOpts := gen.HandshakeOptions{Cookie: "secret", Flags: gen.NetworkFlags{Enable: true, EnableFragmentation: true}, MaxMessageSize: 2000}

	type res struct {
		r   gen.HandshakeResult
		err error
	}
	ch := make(chan res, 1)
	go func() {
		r, err := hs.Accept(acceptor, ac, acceptorOpts)
		ch <- res{r, err}
	}()

	startRes, startErr := hs.Start(dialer, dc, dialerOpts)
	acceptRes := <-ch

	check.NoError(t, startErr)
	check.NoError(t, acceptRes.err)

	// the dialer's view of the acceptor
	check.Equal(t, gen.Atom("acceptor@localhost"), startRes.Peer)
	check.Equal(t, int64(200), startRes.PeerCreation)
	check.Equal(t, acceptorOpts.Flags, startRes.PeerFlags)
	check.Equal(t, 2000, startRes.PeerMaxMessageSize)

	// the acceptor's view of the dialer
	check.Equal(t, gen.Atom("dialer@localhost"), acceptRes.r.Peer)
	check.Equal(t, int64(100), acceptRes.r.PeerCreation)
	check.Equal(t, dialerOpts.Flags, acceptRes.r.PeerFlags)
	check.Equal(t, 1000, acceptRes.r.PeerMaxMessageSize)

	// both agree on the same non-empty connection id
	check.NotEqual(t, "", startRes.ConnectionID)
	check.Equal(t, acceptRes.r.ConnectionID, startRes.ConnectionID)

	// the proto layer receives its connection options
	_, ok := startRes.Custom.(ConnectionOptions)
	check.True(t, ok)
}

// a cookie mismatch fails the digest check on the acceptor and aborts both sides.
func TestHandshakeCookieMismatch(t *testing.T) {
	hs := Create(Options{})
	dc, ac := net.Pipe()
	t.Cleanup(func() { dc.Close() })

	ch := make(chan error, 1)
	go func() {
		_, err := hs.Accept(&hsNode{name: "acceptor@localhost", creation: 2}, ac, gen.HandshakeOptions{Cookie: "wrong"})
		ac.Close()
		ch <- err
	}()

	_, startErr := hs.Start(&hsNode{name: "dialer@localhost", creation: 1}, dc, gen.HandshakeOptions{Cookie: "secret"})
	check.Error(t, startErr)
	check.ErrorContains(t, <-ch, "digest")
}

// a pool join carries the existing connection id; the acceptor matches the join
// digest and adopts the joining node's identity for that connection.
func TestJoinRoundTrip(t *testing.T) {
	hs := Create(Options{})
	dc, ac := net.Pipe()
	t.Cleanup(func() { dc.Close(); ac.Close() })

	opts := gen.HandshakeOptions{Cookie: "secret"}
	type res struct {
		r   gen.HandshakeResult
		err error
	}
	ch := make(chan res, 1)
	go func() {
		r, err := hs.Accept(&hsNode{name: "acceptor@localhost", creation: 2}, ac, opts)
		ch <- res{r, err}
	}()

	_, err := hs.Join(&hsNode{name: "dialer@localhost", creation: 1}, dc, "conn-id-123", opts)
	check.NoError(t, err)

	ar := <-ch
	check.NoError(t, ar.err)
	check.Equal(t, gen.Atom("dialer@localhost"), ar.r.Peer)
	check.Equal(t, "conn-id-123", ar.r.ConnectionID)
}

// Reject writes a MessageReject the other side decodes.
func TestReject(t *testing.T) {
	h := Create(Options{}).(*handshake)
	a, b := net.Pipe()
	go func() {
		h.Reject(a, "denied")
		a.Close()
	}()

	msg, _, err := h.readMessage(b, time.Second, nil)
	check.NoError(t, err)
	rej, ok := msg.(MessageReject)
	check.True(t, ok)
	check.Equal(t, "denied", rej.Reason)
}
