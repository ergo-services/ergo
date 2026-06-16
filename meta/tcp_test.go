package meta

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/mock"
)

// dialConn starts a localhost listener, dials it through CreateTCPConnection and
// returns the connection meta, the accepted server side, and the meta egress
// channel. The listener and server conn are closed on test cleanup.
func dialConn(t *testing.T, opts TCPConnectionOptions) (*tcpconnection, net.Conn, chan any) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	addr := ln.Addr().(*net.TCPAddr)
	opts.Host = "127.0.0.1"
	opts.Port = uint16(addr.Port)

	mb, err := CreateTCPConnection(opts)
	if err != nil {
		t.Fatalf("CreateTCPConnection: %v", err)
	}
	server, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })

	c := mb.(*tcpconnection)
	mp, ch := metaSink()
	c.Init(mp)
	return c, server, ch
}

func readN(t *testing.T, conn net.Conn, n int) []byte {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, n)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read %d bytes: %v", n, err)
	}
	return buf
}

func TestTCPConnectionRoundTrip(t *testing.T) {
	c, server, ch := dialConn(t, TCPConnectionOptions{})

	done := make(chan error, 1)
	go func() { done <- c.Start() }()

	connect := recvMsg[MessageTCPConnect](t, ch)
	if connect.RemoteAddr == nil || connect.LocalAddr == nil {
		t.Fatal("MessageTCPConnect must carry both addresses")
	}

	// inbound: bytes written by the peer surface as MessageTCP
	server.Write([]byte("hello"))
	if got := recvMsg[MessageTCP](t, ch); string(got.Data) != "hello" {
		t.Fatalf("inbound = %q, want hello", got.Data)
	}

	// outbound: HandleMessage writes back to the peer
	if err := c.HandleMessage(gen.PID{}, MessageTCP{Data: []byte("pong")}); err != nil {
		t.Fatal(err)
	}
	if got := readN(t, server, 4); string(got) != "pong" {
		t.Fatalf("outbound = %q, want pong", got)
	}

	// HandleMessage ignores unsupported types, HandleCall is a no-op
	if err := c.HandleMessage(gen.PID{}, 42); err != nil {
		t.Fatal(err)
	}
	if _, err := c.HandleCall(gen.PID{}, gen.Ref{}, "x"); err != nil {
		t.Fatal(err)
	}

	insp := c.HandleInspect(gen.PID{})
	if insp["remote"] == "" || insp["local"] == "" {
		t.Fatalf("inspect missing addrs: %v", insp)
	}

	// closing the peer ends Start and yields MessageTCPDisconnect
	server.Close()
	recvMsg[MessageTCPDisconnect](t, ch)
	if err := <-done; err != nil {
		t.Fatalf("Start: %v", err)
	}
	c.Terminate(nil)
}

func TestTCPConnectionChunkMode(t *testing.T) {
	c, server, ch := dialConn(t, TCPConnectionOptions{
		ReadChunk: ChunkOptions{Enable: true, FixedLength: 4},
	})

	go c.Start()
	recvMsg[MessageTCPConnect](t, ch)

	server.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	if got := recvMsg[MessageTCP](t, ch); string(got.Data) != string([]byte{1, 2, 3, 4}) {
		t.Fatalf("chunk1 = %v", got.Data)
	}
	if got := recvMsg[MessageTCP](t, ch); string(got.Data) != string([]byte{5, 6, 7, 8}) {
		t.Fatalf("chunk2 = %v", got.Data)
	}
	server.Close()
}

func TestTCPConnectionReadDataWithPool(t *testing.T) {
	pool := &sync.Pool{New: func() any { return make([]byte, 0, 64) }}
	c, server, ch := dialConn(t, TCPConnectionOptions{ReadBufferPool: pool})

	go c.Start()
	recvMsg[MessageTCPConnect](t, ch)

	server.Write([]byte("abc"))
	got := recvMsg[MessageTCP](t, ch)
	if string(got.Data) != "abc" {
		t.Fatalf("inbound = %q, want abc", got.Data)
	}

	// writing back recycles the buffer into the pool
	if err := c.HandleMessage(gen.PID{}, MessageTCP{Data: got.Data}); err != nil {
		t.Fatal(err)
	}
	if out := readN(t, server, 3); string(out) != "abc" {
		t.Fatalf("outbound = %q, want abc", out)
	}
	server.Close()
}

func TestCreateTCPConnectionErrors(t *testing.T) {
	// invalid chunk options never touch the network
	if _, err := CreateTCPConnection(TCPConnectionOptions{
		ReadChunk: ChunkOptions{Enable: true},
	}); err == nil {
		t.Fatal("invalid ReadChunk must fail")
	}

	// dialing a closed port fails
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	ln.Close()
	if _, err := CreateTCPConnection(TCPConnectionOptions{
		Host: "127.0.0.1",
		Port: uint16(addr.Port),
	}); err == nil {
		t.Fatal("dialing a closed port must fail")
	}
}

func TestTCPConnectionTerminateReasons(t *testing.T) {
	c, _, _ := dialConn(t, TCPConnectionOptions{})
	c.Terminate(gen.TerminateReasonShutdown)

	c2, _, _ := dialConn(t, TCPConnectionOptions{})
	c2.Terminate(gen.ErrTimeout) // abnormal: logged
}

func TestTCPServerAcceptAndLifecycle(t *testing.T) {
	mb, err := CreateTCPServer(TCPServerOptions{Host: "127.0.0.1", Port: 0})
	if err != nil {
		t.Fatalf("CreateTCPServer: %v", err)
	}
	s := mb.(*tcpserver)

	spawned := make(chan struct{}, 1)
	mp := mock.NewMeta()
	// fail the spawn so the server closes the accepted conn and logs, covering
	// the spawn-error branch without leaving a started connection behind
	mp.OnSpawn(func(b gen.MetaBehavior, o gen.MetaOptions) (gen.Alias, error) {
		spawned <- struct{}{}
		return gen.Alias{}, gen.ErrProcessTerminated
	})
	s.Init(mp)

	go s.Start()

	addr := s.listener.Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	select {
	case <-spawned:
	case <-time.After(time.Second):
		t.Fatal("server never accepted/spawned a connection")
	}

	if insp := s.HandleInspect(gen.PID{}); insp["listener"] == "" {
		t.Fatal("inspect missing listener")
	}
	if err := s.HandleMessage(gen.PID{}, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HandleCall(gen.PID{}, gen.Ref{}, "x"); err != nil {
		t.Fatal(err)
	}

	s.Terminate(nil) // closes the listener, Accept fails, Start returns
}
