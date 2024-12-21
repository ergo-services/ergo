package local

import (
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
	return &udp{}
}

type port struct {
	act.Actor
	proc bool
	tc   *testcase
}

func (p *port) Init(args ...any) error {
	p.tc = args[0].(*testcase)
	p.proc = args[1].(bool)
	if p.proc {
		return nil
	}

	// port with no Process
	opt := meta.PortOptions{}
	if metaport, err := meta.CreatePort(opt); err != nil {
		p.Log().Error("unable to create udp meta-process: %s", err)
		return nil
	} else {
		if _, err := p.SpawnMeta(metaport, gen.MetaOptions{}); err != nil {
			p.Log().Error("unable to spawn port meta-process", err)
			metaport.Terminate(err)
			return nil
		}
	}

	// port with Process
	opt = meta.PortOptions{
		Process: "handler",
	}
	if metaport, err := meta.CreatePort(opt); err != nil {
		p.Log().Error("unable to create port meta-process: %s", err)
		return nil
	} else {
		if _, err := p.SpawnMeta(metaport, gen.MetaOptions{}); err != nil {
			p.Log().Error("unable to spawn port meta-process", err)
			metaport.Terminate(err) // to stop the program
			return nil
		}
	}
	return nil
}

func (p *port) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case meta.MessagePortData:
		p.Log().Info("got packet from %s: %s ", m.Tag, string(m.Data))
		if p.proc {
			m.Data = []byte("proc")
		} else {
			m.Data = []byte("noproc")
		}
		if err := p.SendAlias(m.ID, m); err != nil {
			p.Log().Error("unable to send to %s: %s", m.ID, err)
		}
		p.tc.err <- nil
	default:
		p.Log().Info("got unknown message from %s: %#v", from, message)
	}
	return nil
}

func (t *t18) TestPort(input any) {
	defer func() {
		t.testcase = nil
	}()

	tcproc := &testcase{"", nil, nil, make(chan error)}
	procpid, err := t.SpawnRegister("handler", factory_port, gen.ProcessOptions{}, tcproc, true)
	if err != nil {
		t.Log().Error("unable to spawn 'handler' process: %s", err)
		t.testcase.err <- err
		return
	}
	t.Log().Info("spawned port process with name: %s", procpid)

	tcnoproc := &testcase{"", nil, nil, make(chan error)}
	noprocpid, err := t.Spawn(factory_port, gen.ProcessOptions{}, tcnoproc, false)
	if err != nil {
		t.Log().Error("unable to spawn port process: %s", err)
		t.testcase.err <- err
		return
	}
	t.Log().Info("spawned port process with no name and meta port: %s", noprocpid)

	// TODO

	t.testcase.err <- nil
}

func TestT18Port(t *testing.T) {
	nopt := gen.NodeOptions{}
	// nopt.Log.DefaultLogger.Disable = true
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
		{"TestPort", nil, nil, make(chan error)},
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
