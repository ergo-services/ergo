package local

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/meta"
)

var (
	t18cases []*testcase
)

func factory_t18() gen.ProcessBehavior {
	return &t18{}
}

type t18 struct {
	act.Actor

	testcase *testcase
}

func (t *t18) HandleMessage(from gen.PID, message any) error {
	if t.testcase == nil {
		t.testcase = message.(*testcase)
		message = initcase{}
	}
	// get method by name
	method := reflect.ValueOf(t).MethodByName(t.testcase.name)
	if method.IsValid() == false {
		t.testcase.err <- fmt.Errorf("unknown method %q", t.testcase.name)
		t.testcase = nil
		return nil
	}
	method.Call([]reflect.Value{reflect.ValueOf(message)})
	return nil
}

func factory_port() gen.ProcessBehavior {
	return &port{}
}

type port struct {
	act.Actor
	tc *testcase
}

func (p *port) Init(args ...any) error {
	p.tc = args[0].(*testcase)

	opt := meta.PortOptions{
		Cmd: "go",
	}

	binmode := args[1].(bool)
	if binmode == false {
		opt.Args = append(opt.Args, "run", "./port_txt/main.go")
	} else {
		opt.Args = append(opt.Args, "run", "./port_bin/main.go")
		opt.Binary.Enable = true
		opt.Binary.ReadChunk.FixedLength = 4
	}

	metaport, err := meta.CreatePort(opt)
	if err != nil {
		p.Log().Error("unable to create port meta-process: %s", err)
		return err
	}

	id, err := p.SpawnMeta(metaport, gen.MetaOptions{})
	if err != nil {
		p.Log().Error("unable to spawn port meta-process", err)
		metaport.Terminate(err)
		return err
	}

	if binmode == false {
		txt := meta.MessagePortText{
			Text: "T18 text", // ping message
		}
		return p.Send(id, txt)
	}

	d := "T18 binary"
	data := append([]byte{0, 0, 0, 0}, []byte(d)...)
	binary.BigEndian.PutUint32(data[:4], uint32(len(d)))
	bin := meta.MessagePortData{
		Data: data, // ping message
	}
	return p.Send(id, bin)
}

func (p *port) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case meta.MessagePortData:
		p.Log().Info("got bin packet from %s: %s ", m.Tag, string(m.Data))
		if len(m.Data) < 4 {
			p.tc.err <- errIncorrect
			break
		}
		if string(m.Data[4:]) != "T18 binary pong" {
			p.tc.err <- errIncorrect
			break
		}
		p.tc.err <- nil

	case meta.MessagePortText:
		p.Log().Info("got txt packet from %s: %s ", m.Tag, string(m.Text))
		if m.Text != "T18 text pong" {
			p.tc.err <- errIncorrect
			break
		}

		p.tc.err <- nil

	case meta.MessagePortStart:
		// ignore this message
		p.Log().Debug("port started %s", m.ID)
		break

	case meta.MessagePortError: // from stderr
		p.tc.err <- m.Error

	default:
		p.Log().Info("got unknown message from %s: %#v", from, message)
	}
	return nil
}

func (t *t18) TestPortBin(input any) {
	defer func() {
		t.testcase = nil
	}()

	tc := &testcase{"", nil, nil, make(chan error)}
	pid, err := t.Spawn(factory_port, gen.ProcessOptions{}, tc, true)
	if err != nil {
		t.Log().Error("unable to spawn process: %s", err)
		t.testcase.err <- err
		return
	}
	t.Log().Info("spawned process with meta port (bin): %s", pid)

	t.testcase.err <- tc.wait(3)
}

func (t *t18) TestPortTxt(input any) {
	defer func() {
		t.testcase = nil
	}()

	tc := &testcase{"", nil, nil, make(chan error)}
	pid, err := t.Spawn(factory_port, gen.ProcessOptions{}, tc, false)
	if err != nil {
		t.Log().Error("unable to spawn process: %s", err)
		t.testcase.err <- err
		return
	}
	t.Log().Info("spawned process with meta port (txt): %s", pid)
	t.testcase.err <- tc.wait(3)
}

func TTestT18Port(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	nopt.Log.Level = gen.LogLevelTrace
	node, err := ergo.StartNode("t18node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t18, popt)
	if err != nil {
		panic(err)
	}

	t18cases = []*testcase{
		// {"TestPortBin", nil, nil, make(chan error)},
		// {"TestPortTxt", nil, nil, make(chan error)},
	}
	for _, tc := range t18cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(3); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
