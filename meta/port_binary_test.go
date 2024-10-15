package meta

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

type mockMetaProcess struct {
	result chan any
}

func TestPortBinaryWithHeader(t *testing.T) {
	r, w := io.Pipe()
	p := port{
		out: r,
	}
	mp := &mockMetaProcess{
		result: make(chan any),
	}

	p.MetaProcess = mp
	p.binary.Enable = true
	p.binary.ChunkHeaderSize = 3
	p.binary.ChunkHeaderLengthSize = 1
	p.binary.ChunkHeaderLengthPosition = 2
	p.binary.ReadBufferSize = 50

	go func() {
		fmt.Println("starting readStdoutData")
		p.readStdoutData("x")
		fmt.Println("stopped readStdoutData")
	}()

	//            chunk1......  chunk2................  chunk3...........
	buf := []byte{0, 0, 1, 100, 0, 0, 3, 101, 102, 103, 0, 0, 2, 104, 105}
	buf1 := buf[:5]
	buf2 := buf[5:9]
	buf3 := buf[9:]

	fmt.Println("writing", buf1)
	w.Write(buf1) // chunk1 + tail
	fmt.Println("wrote", buf1)
	if err := mp.waitFor([]byte{0, 0, 1, 100}); err != nil {
		panic(err)
	}

	fmt.Println("writing", buf2)
	w.Write(buf2) // not enough for the chunk2
	if err := mp.waitFor([]byte{}); err != gen.ErrTimeout {
		panic("malformed")
	}

	w.Write(buf3) // expecting chunk2 and chunk3
	if err := mp.waitFor([]byte{0, 0, 3, 101, 102, 103}); err != gen.ErrTimeout {
		panic("malformed chunk2")
	}
	if err := mp.waitFor([]byte{0, 0, 2, 104, 105}); err != gen.ErrTimeout {
		panic("malformed chunk3")
	}
	if err := mp.waitFor([]byte{}); err != gen.ErrTimeout {
		panic("malformed. must be timeout here")
	}
}

func (m *mockMetaProcess) waitFor(expecting []byte) error {
	select {
	case r := <-m.result:
		switch res := r.(type) {
		case MessagePortData:
			if bytes.Compare(res.Data, expecting) != 0 {
				fmt.Printf("got incorrect data (expected %#v): %#v\n", expecting, res.Data)
				return gen.ErrMalformed
			}
			return nil
		default:
			fmt.Printf("got incorrect result (expected MessagePortData): %#v\n", r)
			return gen.ErrIncorrect
		}
	case <-time.After(100 * time.Millisecond):
		return gen.ErrTimeout

	}
}

//
// Mock interfaces
//

func (m *mockMetaProcess) ID() gen.Alias   { return gen.Alias{} }
func (m *mockMetaProcess) Parent() gen.PID { return gen.PID{} }
func (m *mockMetaProcess) Send(to any, message any) error {
	select {
	case m.result <- message:
	default:
		panic("no reader")
	}
	return nil
}
func (m *mockMetaProcess) SendImportant(to any, message any) error { return nil }
func (m *mockMetaProcess) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	return nil
}
func (m *mockMetaProcess) Spawn(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error) {
	return gen.Alias{}, nil
}
func (m *mockMetaProcess) Env(name gen.Env) (any, bool) { return nil, false }
func (m *mockMetaProcess) EnvList() map[gen.Env]any     { return nil }
func (m *mockMetaProcess) Log() gen.Log                 { return &mockLog{} }

type mockLog struct{}

func (l *mockLog) Level() gen.LogLevel                { return gen.LogLevelInfo }
func (l *mockLog) SetLevel(level gen.LogLevel) error  { return nil }
func (l *mockLog) Logger() string                     { return "" }
func (l *mockLog) SetLogger(name string)              {}
func (l *mockLog) Trace(format string, args ...any)   {}
func (l *mockLog) Debug(format string, args ...any)   {}
func (l *mockLog) Info(format string, args ...any)    {}
func (l *mockLog) Warning(format string, args ...any) {}
func (l *mockLog) Error(format string, args ...any)   { panic(fmt.Sprintf(format, args...)) }
func (l *mockLog) Panic(format string, args ...any)   { panic(fmt.Sprintf(format, args...)) }
