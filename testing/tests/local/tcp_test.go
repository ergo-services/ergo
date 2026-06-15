package local

import (
	"encoding/binary"
	"errors"
	"net"
	"strconv"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/meta"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// tcpEvent is what a TCP-handling actor reports to the collector for each TCP
// event it receives, so the events are observable as Send records (a meta
// delivering to its own parent is a self-send that bypasses the core recorder,
// so the receiving actor re-reports instead).
type tcpEvent struct {
	Kind string // connect | data | disconnect
	Data string
}

// tcpReporter is a pool handler: it reports each TCP event to the collector.
type tcpReporter struct {
	act.Actor
	collector gen.PID
}

func factoryTcpReporter() gen.ProcessBehavior { return &tcpReporter{} }

func (r *tcpReporter) Init(args ...any) error {
	r.collector = args[0].(gen.PID)
	return nil
}

func (r *tcpReporter) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case meta.MessageTCPConnect:
		return r.Send(r.collector, tcpEvent{Kind: "connect"})
	case meta.MessageTCPDisconnect:
		return r.Send(r.collector, tcpEvent{Kind: "disconnect"})
	case meta.MessageTCP:
		return r.Send(r.collector, tcpEvent{Kind: "data", Data: string(m.Data)})
	}
	return nil
}

// tcpOwner spawns two TCP servers on ephemeral ports: one with no pool (its own
// connections are delivered to the owner) and one with ProcessPool
// [handler1, handler2] (connections round-robin to those processes). It reports
// its own TCP events to the collector and replies "noproc" to a data message.
type tcpOwner struct {
	act.Actor
	collector    gen.PID
	serverNoPool gen.Alias
	serverPool   gen.Alias
}

func factoryTcpOwner() gen.ProcessBehavior { return &tcpOwner{} }

func (o *tcpOwner) Init(args ...any) error {
	o.collector = args[0].(gen.PID)

	s1, err := meta.CreateTCPServer(meta.TCPServerOptions{Host: "127.0.0.1", Port: 0})
	if err != nil {
		return err
	}
	if o.serverNoPool, err = o.SpawnMeta(s1, gen.MetaOptions{}); err != nil {
		return err
	}

	s2, err := meta.CreateTCPServer(meta.TCPServerOptions{
		Host:        "127.0.0.1",
		Port:        0,
		ProcessPool: []gen.Atom{"handler1", "handler2"},
	})
	if err != nil {
		return err
	}
	if o.serverPool, err = o.SpawnMeta(s2, gen.MetaOptions{}); err != nil {
		return err
	}
	return nil
}

func (o *tcpOwner) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request {
	case "addr_nopool":
		insp, err := o.InspectMeta(o.serverNoPool)
		if err != nil {
			return err, nil
		}
		return insp["listener"], nil
	case "addr_pool":
		insp, err := o.InspectMeta(o.serverPool)
		if err != nil {
			return err, nil
		}
		return insp["listener"], nil
	}
	return "ok", nil
}

func (o *tcpOwner) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case meta.MessageTCPConnect:
		return o.Send(o.collector, tcpEvent{Kind: "connect"})
	case meta.MessageTCPDisconnect:
		return o.Send(o.collector, tcpEvent{Kind: "disconnect"})
	case meta.MessageTCP:
		o.Send(o.collector, tcpEvent{Kind: "data", Data: string(m.Data)})
		m.Data = []byte("noproc")
		return o.SendAlias(m.ID, m)
	}
	return nil
}

// tcpClient owns an outgoing TCP connection meta (Process unset, so its messages
// are delivered to this process); it drives the connection on command and reports
// its TCP events to the collector.
type tcpClient struct {
	act.Actor
	collector gen.PID
	conn      gen.Alias
	port      uint16
}

func factoryTcpClient() gen.ProcessBehavior { return &tcpClient{} }

func (c *tcpClient) Init(args ...any) error {
	c.collector = args[0].(gen.PID)
	c.port = args[1].(uint16)
	return nil
}

func (c *tcpClient) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request {
	case "connect":
		conn, err := meta.CreateTCPConnection(meta.TCPConnectionOptions{Host: "127.0.0.1", Port: c.port})
		if err != nil {
			return err, nil
		}
		id, err := c.SpawnMeta(conn, gen.MetaOptions{})
		if err != nil {
			return err, nil
		}
		c.conn = id
		return id, nil
	case "send":
		return errText(c.SendAlias(c.conn, meta.MessageTCP{Data: []byte("hi")})), nil
	case "close":
		return errText(c.SendExitMeta(c.conn, errors.New("whatever"))), nil
	}
	return "ok", nil
}

func (c *tcpClient) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case meta.MessageTCPConnect:
		return c.Send(c.collector, tcpEvent{Kind: "connect"})
	case meta.MessageTCPDisconnect:
		return c.Send(c.collector, tcpEvent{Kind: "disconnect"})
	case meta.MessageTCP:
		return c.Send(c.collector, tcpEvent{Kind: "data", Data: string(m.Data)})
	}
	return nil
}

// tcpReport asserts that actor `from` reported exactly one event ev since mark.
func tcpReport(t *testing.T, n *stage.Node, mk int, from gen.PID, ev tcpEvent) {
	t.Helper()
	n.ShouldSend().From(from).Message(ev).Since(mk).Once().Within(time.Second).Must()
}

func portOf(t *testing.T, addr string) uint16 {
	t.Helper()
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	v, err := strconv.Atoi(p)
	if err != nil {
		t.Fatal(err)
	}
	return uint16(v)
}

// tcpFrameOwner spawns a TCP server with length-prefixed chunk framing (4-byte
// big-endian payload length header) and reports each delivered frame's payload to
// the collector.
type tcpFrameOwner struct {
	act.Actor
	collector gen.PID
	server    gen.Alias
}

func factoryTcpFrameOwner() gen.ProcessBehavior { return &tcpFrameOwner{} }

func (o *tcpFrameOwner) Init(args ...any) error {
	o.collector = args[0].(gen.PID)
	s, err := meta.CreateTCPServer(meta.TCPServerOptions{
		Host: "127.0.0.1",
		Port: 0,
		ReadChunk: meta.ChunkOptions{
			Enable:               true,
			HeaderSize:           4,
			HeaderLengthPosition: 0,
			HeaderLengthSize:     4,
		},
	})
	if err != nil {
		return err
	}
	o.server, err = o.SpawnMeta(s, gen.MetaOptions{})
	return err
}

func (o *tcpFrameOwner) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == "addr" {
		insp, err := o.InspectMeta(o.server)
		if err != nil {
			return err, nil
		}
		return insp["listener"], nil
	}
	return "ok", nil
}

func (o *tcpFrameOwner) HandleMessage(from gen.PID, message any) error {
	if m, ok := message.(meta.MessageTCP); ok && len(m.Data) >= 4 {
		return o.Send(o.collector, string(m.Data[4:]))
	}
	return nil
}

// frameOf builds a length-prefixed frame: 4-byte big-endian payload length + payload.
func frameOf(payload string) []byte {
	b := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(b[:4], uint32(len(payload)))
	copy(b[4:], payload)
	return b
}

// TestLocalTCPFraming: a TCP server with ReadChunk length-prefix framing delivers
// exactly one MessageTCP per logical frame, reassembling correctly regardless of
// how the bytes are segmented on the wire: a whole frame in one write, a frame
// split across writes, and two frames coalesced into one write.
func TestLocalTCPFraming(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")

	collector := n.Spawn(factoryEcho, gen.ProcessOptions{})
	owner := n.Spawn(factoryTcpFrameOwner, gen.ProcessOptions{}, collector)

	addrAny, err := n.Call(owner, "addr")
	check.NoError(t, err)
	conn, err := net.Dial("tcp", addrAny.(string))
	check.NoError(t, err)
	defer conn.Close()
	conn.(*net.TCPConn).SetNoDelay(true) // each Write is its own segment

	// a whole frame in one write -> one frame
	mk := n.Mark()
	_, err = conn.Write(frameOf("AAA"))
	check.NoError(t, err)
	n.ShouldSend().From(owner).Message("AAA").Since(mk).Once().Within(time.Second).Must()

	// a frame split across two writes -> still one reassembled frame
	mk = n.Mark()
	f := frameOf("BBBB")
	_, err = conn.Write(f[:3]) // partial header+payload
	check.NoError(t, err)
	time.Sleep(20 * time.Millisecond) // force a separate segment so the read splits
	_, err = conn.Write(f[3:])
	check.NoError(t, err)
	n.ShouldSend().From(owner).Message("BBBB").Since(mk).Once().Within(time.Second).Must()

	// two frames coalesced in one write -> two frames, in order
	mk = n.Mark()
	two := append(frameOf("CC"), frameOf("DDD")...)
	_, err = conn.Write(two)
	check.NoError(t, err)
	got := n.ShouldSend().From(owner).Since(mk).Times(2).Within(time.Second).Collect()
	check.Equal(t, 2, len(got))
	check.Equal(t, "CC", got[0].Message)
	check.Equal(t, "DDD", got[1].Message)
}

// TestLocalTCPServer: a TCP server with a ProcessPool delivers each accepted
// connection round-robin to the pool processes (connect and disconnect), while a
// server with no pool delivers connections to its owning process, which echoes a
// reply the client reads back over the socket.
func TestLocalTCPServer(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")

	collector := n.Spawn(factoryEcho, gen.ProcessOptions{})
	h1 := n.SpawnRegister("handler1", factoryTcpReporter, gen.ProcessOptions{}, collector)
	h2 := n.SpawnRegister("handler2", factoryTcpReporter, gen.ProcessOptions{}, collector)
	owner := n.Spawn(factoryTcpOwner, gen.ProcessOptions{}, collector)

	addrPoolAny, err := n.Call(owner, "addr_pool")
	check.NoError(t, err)
	addrPool := addrPoolAny.(string)
	addrNoPoolAny, err := n.Call(owner, "addr_nopool")
	check.NoError(t, err)
	addrNoPool := addrNoPoolAny.(string)

	// first connection to the pool server -> handler1
	mk := n.Mark()
	c1, err := net.Dial("tcp", addrPool)
	check.NoError(t, err)
	tcpReport(t, n, mk, h1, tcpEvent{Kind: "connect"})
	mk = n.Mark()
	c1.Close()
	tcpReport(t, n, mk, h1, tcpEvent{Kind: "disconnect"})

	// second connection -> handler2
	mk = n.Mark()
	c2, err := net.Dial("tcp", addrPool)
	check.NoError(t, err)
	tcpReport(t, n, mk, h2, tcpEvent{Kind: "connect"})
	mk = n.Mark()
	c2.Close()
	tcpReport(t, n, mk, h2, tcpEvent{Kind: "disconnect"})

	// connection to the no-pool server -> owner; data echoed back over the socket
	mk = n.Mark()
	conn, err := net.Dial("tcp", addrNoPool)
	check.NoError(t, err)
	tcpReport(t, n, mk, owner, tcpEvent{Kind: "connect"})

	mk = n.Mark()
	_, err = conn.Write([]byte("hi"))
	check.NoError(t, err)
	tcpReport(t, n, mk, owner, tcpEvent{Kind: "data", Data: "hi"})

	conn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 16)
	nr, err := conn.Read(buf)
	check.NoError(t, err)
	check.Equal(t, "noproc", string(buf[:nr]))

	mk = n.Mark()
	conn.Close()
	tcpReport(t, n, mk, owner, tcpEvent{Kind: "disconnect"})
}

// TestLocalTCPClient: an outgoing TCP connection meta delivers connect, data (the
// server's "noproc" reply), and disconnect to its owning process; the server side
// observes the matching connect, the incoming data, and the disconnect.
func TestLocalTCPClient(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")

	collector := n.Spawn(factoryEcho, gen.ProcessOptions{})
	owner := n.Spawn(factoryTcpOwner, gen.ProcessOptions{}, collector)
	addrAny, err := n.Call(owner, "addr_nopool")
	check.NoError(t, err)
	port := portOf(t, addrAny.(string))

	client := n.Spawn(factoryTcpClient, gen.ProcessOptions{}, collector, port)

	// connect: both ends observe a connect
	mk := n.Mark()
	connAny, err := n.Call(client, "connect")
	check.NoError(t, err)
	_, ok := connAny.(gen.Alias)
	check.True(t, ok)
	tcpReport(t, n, mk, client, tcpEvent{Kind: "connect"})
	tcpReport(t, n, mk, owner, tcpEvent{Kind: "connect"})

	// send "hi": the server receives it and replies "noproc", which the client receives
	mk = n.Mark()
	_, err = n.Call(client, "send")
	check.NoError(t, err)
	tcpReport(t, n, mk, owner, tcpEvent{Kind: "data", Data: "hi"})
	tcpReport(t, n, mk, client, tcpEvent{Kind: "data", Data: "noproc"})

	// close: both ends observe a disconnect
	mk = n.Mark()
	_, err = n.Call(client, "close")
	check.NoError(t, err)
	tcpReport(t, n, mk, owner, tcpEvent{Kind: "disconnect"})
	tcpReport(t, n, mk, client, tcpEvent{Kind: "disconnect"})
}
