package distributed

import (
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/meta"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// socketHost owns a TCP server and a UDP server, both delivering to this
// process, and forwards every accepted connection to a process on another node.
type socketHost struct {
	act.Actor
	forward   gen.PID
	tcpServer gen.Alias
	udpServer gen.Alias
	conn      gen.Alias
}

func factorySocketHost() gen.ProcessBehavior { return &socketHost{} }

func (h *socketHost) Init(args ...any) error {
	h.forward = args[0].(gen.PID)

	s, err := meta.CreateTCPServer(meta.TCPServerOptions{Host: "127.0.0.1", Port: 0})
	if err != nil {
		return err
	}
	if h.tcpServer, err = h.SpawnMeta(s, gen.MetaOptions{}); err != nil {
		return err
	}

	u, err := meta.CreateUDPServer(meta.UDPServerOptions{Host: "127.0.0.1", Port: 0})
	if err != nil {
		return err
	}
	h.udpServer, err = h.SpawnMeta(u, gen.MetaOptions{})
	return err
}

func (h *socketHost) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request {
	case "tcp_addr":
		insp, err := h.InspectMeta(h.tcpServer)
		if err != nil {
			return err, nil
		}
		return insp["listener"], nil
	case "udp_addr":
		insp, err := h.InspectMeta(h.udpServer)
		if err != nil {
			return err, nil
		}
		return insp["listener"], nil
	case "udp_alias":
		return h.udpServer, nil
	case "conn":
		return h.conn, nil
	}
	return "ok", nil
}

func (h *socketHost) HandleMessage(from gen.PID, message any) error {
	if m, ok := message.(meta.MessageTCPConnect); ok {
		h.conn = m.ID
		return h.Send(h.forward, m)
	}
	return nil
}

// tcpWriteCmd and udpWriteCmd ask a process to write to a socket owned by a meta
// process on another node, addressing it by the meta process alias.
type tcpWriteCmd struct {
	Conn gen.Alias
	Data []byte
}

type udpWriteCmd struct {
	Server gen.Alias
	Addr   netip.AddrPort
	Data   []byte
}

type socketSink struct{ act.Actor }

func factorySocketSink() gen.ProcessBehavior { return &socketSink{} }

func (s *socketSink) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch c := request.(type) {
	case tcpWriteCmd:
		return errText(s.Send(c.Conn, meta.MessageTCP{ID: c.Conn, Data: c.Data})), nil
	case udpWriteCmd:
		return errText(s.Send(c.Server, meta.MessageUDP{ID: c.Server, Addr: c.Addr, Data: c.Data})), nil
	}
	return "ok", nil
}

func (s *socketSink) HandleMessage(from gen.PID, message any) error { return nil }

// TestDistMetaSocket: the TCP and UDP meta messages are network capable. The
// connect message forwarded to another node keeps both addresses, and a process
// on that node writes to the socket by the meta process alias: into the accepted
// TCP connection, and as a datagram through the UDP server.
func TestDistMetaSocket(t *testing.T) {
	s := stage.New(t)
	n1 := s.StartNode("n1")
	n2 := s.StartNode("n2")
	s.Connect(n1, n2)

	sink := n2.Spawn(factorySocketSink, gen.ProcessOptions{})
	host := n1.Spawn(factorySocketHost, gen.ProcessOptions{}, sink)

	v, err := n1.Call(host, "tcp_addr")
	check.NoError(t, err)
	tcpAddr := v.(string)

	mk := n2.Mark()
	client, err := net.Dial("tcp", tcpAddr)
	check.NoError(t, err)
	defer client.Close()

	rec, ok := n2.ShouldDeliver().To(sink).Since(mk).
		Where(func(r check.Delivered) bool {
			_, is := r.Message.(meta.MessageTCPConnect)
			return is
		}).Within(5 * time.Second).Capture()
	check.True(t, ok)
	connect := rec.Message.(meta.MessageTCPConnect)

	v, err = n1.Call(host, "conn")
	check.NoError(t, err)
	check.Equal(t, v.(gen.Alias), connect.ID)
	check.Equal(t, netip.MustParseAddrPort(client.LocalAddr().String()), connect.RemoteAddr)
	check.Equal(t, netip.MustParseAddrPort(tcpAddr), connect.LocalAddr)

	// the process on n2 writes into the TCP connection accepted on n1
	v, err = n2.Call(sink, tcpWriteCmd{Conn: connect.ID, Data: []byte("from n2")})
	check.NoError(t, err)
	check.Equal(t, "", v)

	client.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, len("from n2"))
	_, err = io.ReadFull(client, buf)
	check.NoError(t, err)
	check.Equal(t, "from n2", string(buf))

	// the process on n2 sends a datagram through the UDP server on n1
	v, err = n1.Call(host, "udp_alias")
	check.NoError(t, err)
	udpServer := v.(gen.Alias)

	peer, err := net.ListenPacket("udp", "127.0.0.1:0")
	check.NoError(t, err)
	defer peer.Close()

	v, err = n2.Call(sink, udpWriteCmd{
		Server: udpServer,
		Addr:   netip.MustParseAddrPort(peer.LocalAddr().String()),
		Data:   []byte("datagram from n2"),
	})
	check.NoError(t, err)
	check.Equal(t, "", v)

	peer.SetReadDeadline(time.Now().Add(5 * time.Second))
	datagram := make([]byte, 64)
	nr, raddr, err := peer.ReadFrom(datagram)
	check.NoError(t, err)
	check.Equal(t, "datagram from n2", string(datagram[:nr]))

	v, err = n1.Call(host, "udp_addr")
	check.NoError(t, err)
	check.Equal(t, v.(string), raddr.String())
}
