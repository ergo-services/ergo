package meta

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/mock"
)

// bufWriteCloser is an io.WriteCloser backed by a bytes.Buffer, standing in for
// the subprocess stdin in HandleMessage tests.
type bufWriteCloser struct{ buf bytes.Buffer }

func (b *bufWriteCloser) Write(p []byte) (int, error) { return b.buf.Write(p) }
func (b *bufWriteCloser) Close() error                { return nil }

func TestCreatePort(t *testing.T) {
	if _, err := CreatePort(PortOptions{}); err == nil {
		t.Fatal("empty Cmd must fail")
	}

	// non-binary: returned as is
	if _, err := CreatePort(PortOptions{Cmd: "echo"}); err != nil {
		t.Fatalf("plain port: %v", err)
	}

	// binary: default buffer size applied, valid
	if _, err := CreatePort(PortOptions{Cmd: "echo", Binary: PortBinaryOptions{Enable: true}}); err != nil {
		t.Fatalf("binary port: %v", err)
	}

	// keepalive enabled with zero period
	_, err := CreatePort(PortOptions{Cmd: "echo", Binary: PortBinaryOptions{
		Enable:               true,
		WriteBufferKeepAlive: []byte{0},
	}})
	if err == nil || strings.Contains(err.Error(), "zero Period") == false {
		t.Fatalf("got %v, want zero Period error", err)
	}

	// invalid chunk options propagate
	_, err = CreatePort(PortOptions{Cmd: "echo", Binary: PortBinaryOptions{
		Enable:    true,
		ReadChunk: ChunkOptions{Enable: true},
	}})
	if err == nil {
		t.Fatal("invalid ReadChunk must fail")
	}

	// pool of the wrong element type
	badPool := &sync.Pool{New: func() any { return "not bytes" }}
	_, err = CreatePort(PortOptions{Cmd: "echo", Binary: PortBinaryOptions{
		Enable:         true,
		ReadBufferPool: badPool,
	}})
	if err == nil || strings.Contains(err.Error(), "pool of []byte") == false {
		t.Fatalf("got %v, want bad pool error", err)
	}

	// pool of the right type is accepted
	goodPool := &sync.Pool{New: func() any { return make([]byte, 0, 8) }}
	if _, err := CreatePort(PortOptions{Cmd: "echo", Binary: PortBinaryOptions{
		Enable:         true,
		ReadBufferPool: goodPool,
	}}); err != nil {
		t.Fatalf("good pool: %v", err)
	}
}

func TestPortInitAndTerminate(t *testing.T) {
	mp := mock.NewMeta()
	mp.OnEnvList(func() map[gen.Env]any { return map[gen.Env]any{"A": "b"} })

	p := &port{options: PortOptions{
		Cmd:           "echo",
		Args:          []string{"x"},
		EnableEnvOS:   true,
		EnableEnvMeta: true,
		Env:           map[gen.Env]string{"C": "d"},
		Binary:        PortBinaryOptions{Enable: true},
	}}
	if err := p.Init(mp); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if p.cmd == nil || p.in == nil || p.out == nil || p.errout == nil {
		t.Fatal("Init must wire cmd and pipes")
	}
	// the process was never started: Terminate closes the pipes and returns cleanly
	p.Terminate(nil)
}

func TestPortTerminateReasons(t *testing.T) {
	mp := mock.NewMeta()

	// no cmd, normal shutdown reasons are silent
	(&port{MetaProcess: mp}).Terminate(nil)
	(&port{MetaProcess: mp}).Terminate(gen.TerminateReasonShutdown)
	(&port{MetaProcess: mp}).Terminate(gen.TerminateReasonNormal)

	// abnormal reason is logged (dumb logger, no panic)
	(&port{MetaProcess: mp}).Terminate(gen.ErrTimeout)
}

func TestPortHandleMessage(t *testing.T) {
	in := &bufWriteCloser{}
	p := &port{MetaProcess: mock.NewMeta(), in: in}

	if err := p.HandleMessage(gen.PID{}, MessagePortText{Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := p.HandleMessage(gen.PID{}, MessagePortData{Data: []byte("!!")}); err != nil {
		t.Fatal(err)
	}
	if in.buf.String() != "hello!!" {
		t.Fatalf("stdin = %q, want hello!!", in.buf.String())
	}
	if got := atomic.LoadUint64(&p.bytesOut); got != 7 {
		t.Fatalf("bytesOut = %d, want 7", got)
	}

	// unsupported type is ignored (logged), no error
	if err := p.HandleMessage(gen.PID{}, 123); err != nil {
		t.Fatalf("unsupported message: %v", err)
	}

	// HandleCall is a no-op for the port
	if _, err := p.HandleCall(gen.PID{}, gen.Ref{}, "x"); err != nil {
		t.Fatal(err)
	}
}

func TestPortHandleMessageReturnsWriteError(t *testing.T) {
	boom := io.ErrClosedPipe
	p := &port{MetaProcess: mock.NewMeta(), in: errWriteCloser{err: boom}}
	if err := p.HandleMessage(gen.PID{}, MessagePortData{Data: []byte("x")}); err != boom {
		t.Fatalf("got %v, want %v", err, boom)
	}
}

// A stderr line must not be treated as a printf format string: a line with %
// verbs is carried into MessagePortError.Error verbatim.
func TestPortReadStderrErrorTextIsLiteral(t *testing.T) {
	mp := mock.NewMeta()
	var got error
	mp.OnSend(func(to any, message any) error {
		if m, ok := message.(MessagePortError); ok {
			got = m.Error
		}
		return nil
	})

	line := "fatal: %s not found (100% cpu)"
	p := &port{MetaProcess: mp, errout: io.NopCloser(strings.NewReader(line + "\n"))}
	p.readStderr(gen.PID{})

	if got == nil {
		t.Fatal("expected a MessagePortError to be sent")
	}
	if got.Error() != line {
		t.Fatalf("stderr text mangled as a format string: got %q, want %q", got.Error(), line)
	}
}

// errWriteCloser fails every write, exercising the port write-error path.
type errWriteCloser struct{ err error }

func (e errWriteCloser) Write(p []byte) (int, error) { return 0, e.err }
func (e errWriteCloser) Close() error                { return nil }

func TestPortReadStdoutData(t *testing.T) {
	r, w := io.Pipe()
	mp, ch := metaSink()
	p := &port{MetaProcess: mp, out: r, options: PortOptions{
		Binary: PortBinaryOptions{Enable: true, ReadBufferSize: 64},
	}}

	go p.readStdoutData("to")

	w.Write([]byte{1, 2, 3})
	if got := recvMsg[MessagePortData](t, ch); bytes.Equal(got.Data, []byte{1, 2, 3}) == false {
		t.Fatalf("data = %v, want [1 2 3]", got.Data)
	}
	w.Close()
}

func TestPortReadStdoutDataWithPool(t *testing.T) {
	pool := &sync.Pool{New: func() any { return make([]byte, 0, 64) }}
	r, w := io.Pipe()
	mp, ch := metaSink()
	p := &port{MetaProcess: mp, out: r, options: PortOptions{
		Binary: PortBinaryOptions{Enable: true, ReadBufferSize: 64, ReadBufferPool: pool},
	}}

	go p.readStdoutData("to")

	w.Write([]byte{9, 8, 7})
	if got := recvMsg[MessagePortData](t, ch); bytes.Equal(got.Data, []byte{9, 8, 7}) == false {
		t.Fatalf("data = %v, want [9 8 7]", got.Data)
	}
	w.Close()
}

func TestPortReadStdoutText(t *testing.T) {
	r, w := io.Pipe()
	mp, ch := metaSink()
	p := &port{MetaProcess: mp, out: r}

	go p.readStdoutText("to")

	w.Write([]byte("line one\nline two\n"))
	if got := recvMsg[MessagePortText](t, ch); got.Text != "line one" {
		t.Fatalf("text = %q, want line one", got.Text)
	}
	if got := recvMsg[MessagePortText](t, ch); got.Text != "line two" {
		t.Fatalf("text = %q, want line two", got.Text)
	}
	w.Close()
}

func TestPortReadStderr(t *testing.T) {
	r, w := io.Pipe()
	mp, ch := metaSink()
	p := &port{MetaProcess: mp, errout: r}

	go p.readStderr("to")

	w.Write([]byte("oops\n"))
	got := recvMsg[MessagePortError](t, ch)
	if got.Error == nil || got.Error.Error() != "oops" {
		t.Fatalf("error = %v, want oops", got.Error)
	}
	w.Close()
}

// TestPortStartEcho runs a real, instantly-exiting subprocess to exercise the
// text-mode Start/readStdout/readStderr path and HandleInspect end to end.
func TestPortStartEcho(t *testing.T) {
	mp, ch := metaSink()
	p := &port{options: PortOptions{Cmd: "echo", Args: []string{"hello"}}}
	if err := p.Init(mp); err != nil {
		t.Fatalf("Init: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- p.Start() }()

	recvMsg[MessagePortStart](t, ch)
	if got := recvMsg[MessagePortText](t, ch); got.Text != "hello" {
		t.Fatalf("text = %q, want hello", got.Text)
	}
	if err := <-done; err != nil {
		t.Fatalf("Start: %v", err)
	}

	insp := p.HandleInspect(gen.PID{})
	if insp["cmd"] != "echo" {
		t.Fatalf("inspect cmd = %q, want echo", insp["cmd"])
	}
}
