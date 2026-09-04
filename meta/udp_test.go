package meta

import (
	"net"
	"sync"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/mock"
)

func TestUDPServerRoundTrip(t *testing.T) {
	mb, err := CreateUDPServer(UDPServerOptions{Host: "127.0.0.1", Port: 0})
	if err != nil {
		t.Fatalf("CreateUDPServer: %v", err)
	}
	u := mb.(*udpserver)
	mp, ch := metaSink()
	u.Init(mp)

	go u.Start()

	client, err := net.Dial("udp", u.pc.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	got := recvMsg[MessageUDP](t, ch)
	if string(got.Data) != "ping" || got.Addr == nil {
		t.Fatalf("inbound = %q addr %v", got.Data, got.Addr)
	}

	// echo back to the sender
	if err := u.HandleMessage(gen.PID{}, MessageUDP{Data: []byte("pong"), Addr: got.Addr}); err != nil {
		t.Fatal(err)
	}
	client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 4)
	if _, err := client.Read(buf); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(buf) != "pong" {
		t.Fatalf("outbound = %q, want pong", buf)
	}

	// unsupported message is ignored, HandleCall is a no-op
	if err := u.HandleMessage(gen.PID{}, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := u.HandleCall(gen.PID{}, gen.Ref{}, "x"); err != nil {
		t.Fatal(err)
	}

	if insp := u.HandleInspect(gen.PID{}); insp["listener"] == "" {
		t.Fatalf("inspect missing listener: %v", insp)
	}

	u.Terminate(nil) // closes the socket, Start's ReadFrom fails and returns
}

func TestCreateUDPServerBadPool(t *testing.T) {
	badPool := &sync.Pool{New: func() any { return "not bytes" }}
	_, err := CreateUDPServer(UDPServerOptions{Host: "127.0.0.1", Port: 0, BufferPool: badPool})
	if err == nil {
		t.Fatal("a pool of the wrong type must fail")
	}
}

// a recycled pool buffer must be reset to the full size before each read, so a
// short buffer left in the pool does not truncate later datagrams.
func TestUDPServerBufferPoolNoTruncate(t *testing.T) {
	pool := &sync.Pool{New: func() any { return make([]byte, 16)[:4] }} // len 4, cap 16
	mb, err := CreateUDPServer(UDPServerOptions{Host: "127.0.0.1", Port: 0, BufferSize: 16, BufferPool: pool})
	if err != nil {
		t.Fatalf("CreateUDPServer: %v", err)
	}
	u := mb.(*udpserver)
	mp, ch := metaSink()
	u.Init(mp)
	go u.Start()
	defer u.Terminate(nil)

	client, err := net.Dial("udp", u.pc.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if _, err := client.Write([]byte("12345678")); err != nil {
		t.Fatal(err)
	}
	got := recvMsg[MessageUDP](t, ch)
	if string(got.Data) != "12345678" {
		t.Fatalf("datagram truncated: got %q, want 12345678", got.Data)
	}
}

func TestUDPServerTerminateAbnormal(t *testing.T) {
	mb, err := CreateUDPServer(UDPServerOptions{Host: "127.0.0.1", Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	u := mb.(*udpserver)
	u.Init(mock.NewMeta())
	u.Terminate(gen.ErrTimeout) // abnormal reason is logged
}
