package local

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/meta"
	"ergo.services/ergo/testing/stage"
)

// TestPortHelper is the external program for the port test: it is this very test
// binary re-executed (no separate program, no compilation). When the helper env
// var is set it reads one stdin message, writes "<payload> pong" back, and exits
// before the test framework can print anything to stdout. When the var is unset
// (a normal test run) it is a no-op.
func TestPortHelper(t *testing.T) {
	mode := os.Getenv("ERGO_PORT_HELPER")
	if mode == "" {
		return
	}
	buf := make([]byte, 1024)
	n, err := os.Stdin.Read(buf)
	if err != nil {
		os.Exit(1)
	}
	if mode == "bin" {
		l := int(binary.BigEndian.Uint32(buf[:4]))
		resp := string(buf[4:4+l]) + " pong"
		out := make([]byte, 4+len(resp))
		binary.BigEndian.PutUint32(out[:4], uint32(len(resp)))
		copy(out[4:], resp)
		os.Stdout.Write(out)
	} else {
		text := strings.TrimRight(string(buf[:n]), "\r\n")
		fmt.Fprintln(os.Stdout, text+" pong")
	}
	os.Exit(0)
}

// portOwner spawns a port meta running this test binary as the external program
// and pings it, reporting the reply to the collector (a port delivers to its own
// parent via a self-send, which bypasses the core recorder, so the owner
// re-reports).
type portOwner struct {
	act.Actor
	collector gen.PID
	binary    bool
}

func factoryPortOwner() gen.ProcessBehavior { return &portOwner{} }

func (o *portOwner) Init(args ...any) error {
	o.collector = args[0].(gen.PID)
	o.binary = args[1].(bool)
	chunk := args[2].(bool)

	opt := meta.PortOptions{
		Cmd:         os.Args[0],
		Args:        []string{"-test.run=^TestPortHelper$"},
		EnableEnvOS: true,
	}
	if o.binary {
		opt.Env = map[gen.Env]string{"ERGO_PORT_HELPER": "bin"}
		opt.Binary.Enable = true
		if chunk {
			opt.Binary.ReadChunk.Enable = true
			opt.Binary.ReadChunk.HeaderSize = 4
			opt.Binary.ReadChunk.HeaderLengthPosition = 0
			opt.Binary.ReadChunk.HeaderLengthSize = 4
		}
	} else {
		opt.Env = map[gen.Env]string{"ERGO_PORT_HELPER": "txt"}
	}

	mp, err := meta.CreatePort(opt)
	if err != nil {
		return err
	}
	id, err := o.SpawnMeta(mp, gen.MetaOptions{})
	if err != nil {
		return err
	}

	if o.binary {
		d := "hello"
		data := make([]byte, 4+len(d))
		binary.BigEndian.PutUint32(data[:4], uint32(len(d)))
		copy(data[4:], d)
		return o.Send(id, meta.MessagePortData{Data: data})
	}
	return o.Send(id, meta.MessagePortText{Text: "hello"})
}

func (o *portOwner) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case meta.MessagePortText:
		return o.Send(o.collector, m.Text)
	case meta.MessagePortData:
		if len(m.Data) >= 4 {
			return o.Send(o.collector, string(m.Data[4:]))
		}
	case meta.MessagePortError:
		return o.Send(o.collector, "ERR:"+m.Error.Error())
	}
	return nil
}

// TestLocalPort: a port meta runs an external program and exchanges messages with
// it over stdin/stdout, in text mode (line framing), binary mode with chunk
// framing (4-byte big-endian length prefix), and binary mode without chunk framing
// (raw reads delivered as MessagePortData). The external program is this test
// binary re-executed, so the test needs no separate helper program.
func TestLocalPort(t *testing.T) {
	t.Run("Text", func(t *testing.T) {
		s := stage.New(t)
		n := s.Node("n")
		collector := n.Spawn(factoryEcho)
		owner := n.Spawn(factoryPortOwner, collector, false, false)
		n.ShouldSend().From(owner).Message("hello pong").Once().Within(10 * time.Second).Must()
	})

	t.Run("BinaryChunk", func(t *testing.T) {
		s := stage.New(t)
		n := s.Node("n")
		collector := n.Spawn(factoryEcho)
		owner := n.Spawn(factoryPortOwner, collector, true, true)
		n.ShouldSend().From(owner).Message("hello pong").Once().Within(10 * time.Second).Must()
	})

	t.Run("BinaryRaw", func(t *testing.T) {
		s := stage.New(t)
		n := s.Node("n")
		collector := n.Spawn(factoryEcho)
		owner := n.Spawn(factoryPortOwner, collector, true, false)
		n.ShouldSend().From(owner).Message("hello pong").Once().Within(10 * time.Second).Must()
	})
}
