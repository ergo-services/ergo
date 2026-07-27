package handshake

import (
	"fmt"
	"net"
	"testing"

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

// acceptFull drives the two-step acceptor handshake (Negotiate then Accept) as
// the node does, so tests can match it against a single Start on the dialer.
func acceptFull(hs gen.NetworkHandshake, node gen.NodeHandshake, conn net.Conn, opts gen.HandshakeOptions) (gen.HandshakeResult, error) {
	r, err := hs.Negotiate(node, conn, opts)
	if err != nil {
		return r, err
	}
	return hs.Accept(node, conn, opts, r)
}

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
		r, err := acceptFull(hs, acceptor, ac, acceptorOpts)
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
		_, err := acceptFull(hs, &hsNode{name: "acceptor@localhost", creation: 2}, ac, gen.HandshakeOptions{Cookie: "wrong"})
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
		r, err := acceptFull(hs, &hsNode{name: "acceptor@localhost", creation: 2}, ac, opts)
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

	msg, _, err := h.readMessage(b, nil, handshakeMaxControlSize)
	check.NoError(t, err)
	rej, ok := msg.(MessageReject)
	check.True(t, ok)
	check.Equal(t, "denied", rej.Reason)
}

// TestHandshakeIntroduceOverUint16 guards #290: an Introduce whose cache exchange exceeds
// the old 64KB (uint16) ceiling must still round-trip, since the writer frames the length
// as uint32. The same frame read under the small control ceiling is still rejected.
func TestHandshakeIntroduceOverUint16(t *testing.T) {
	h := Create(Options{}).(*handshake)

	// a RegCache large enough that the encoded Introduce exceeds 64KB
	reg := make(map[uint16]string, 3000)
	for i := 0; i < 3000; i++ {
		reg[uint16(i)] = fmt.Sprintf("ergo.services/ergo/net/handshake/testtype/VeryLongTypeName_%05d", i)
	}
	intro := MessageIntroduce{Node: "big@localhost", RegCache: reg}

	// round-trips when read under the configurable Introduce ceiling
	a, b := net.Pipe()
	go func() { h.writeMessage(a, intro); a.Close() }()
	msg, _, err := h.readMessage(b, nil, gen.DefaultHandshakeMaxMessageSize)
	check.NoError(t, err)
	got, ok := msg.(MessageIntroduce)
	check.True(t, ok)
	check.Equal(t, len(reg), len(got.RegCache))

	// the same frame is rejected under the small control ceiling
	c, d := net.Pipe()
	go func() { h.writeMessage(c, intro); c.Close() }()
	_, _, err = h.readMessage(d, nil, handshakeMaxControlSize)
	check.Error(t, err)
	check.ErrorContains(t, err, "too long")
}
