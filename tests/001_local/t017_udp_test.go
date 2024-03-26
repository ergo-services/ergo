package local

import (
	"fmt"
	"net"
	"reflect"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/meta"
)

var (
	t17cases []*testcase
)

func factory_t17() gen.ProcessBehavior {
	return &t17{}
}

type t17 struct {
	act.Actor

	testcase *testcase
}

func (t *t17) HandleMessage(from gen.PID, message any) error {
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

func factory_udp() gen.ProcessBehavior {
	return &udp{}
}

type udp struct {
	act.Actor
	proc bool
	tc   *testcase
}

func (u *udp) Init(args ...any) error {
	u.tc = args[0].(*testcase)
	u.proc = args[1].(bool)
	if u.proc {
		return nil
	}

	// UDP server with no Process
	opt := meta.UDPOptions{
		Port: 17171,
	}
	if metaudp, err := meta.CreateUDP(opt); err != nil {
		u.Log().Error("unable to create udp meta-process: %s", err)
		return nil
	} else {
		if _, err := u.SpawnMeta(metaudp, gen.MetaOptions{}); err != nil {
			u.Log().Error("unable to spawn udp meta-process", err)
			metaudp.Terminate(err) // to stop UDP-listener
			return nil
		}
	}

	// UDP server with Process
	opt = meta.UDPOptions{
		Port:    18181,
		Process: "handler",
	}
	if metaudp, err := meta.CreateUDP(opt); err != nil {
		u.Log().Error("unable to create udp meta-process: %s", err)
		return nil
	} else {
		if _, err := u.SpawnMeta(metaudp, gen.MetaOptions{}); err != nil {
			u.Log().Error("unable to spawn udp meta-process", err)
			metaudp.Terminate(err) // to stop UDP-listener
			return nil
		}
	}
	return nil
}

func (u *udp) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case meta.MessageUDP:
		u.Log().Info("got udp packet from %s: %s ", m.Addr, string(m.Data))
		if u.proc {
			m.Data = []byte("proc")
		} else {
			m.Data = []byte("noproc")
		}
		if err := u.SendAlias(m.ID, m); err != nil {
			u.Log().Error("unable to send to %s: %s", m.ID, err)
		}
		u.tc.err <- nil
	default:
		u.Log().Info("got unknown message from %s: %#v", from, message)
	}
	return nil
}

func (t *t17) TestUDP(input any) {
	defer func() {
		t.testcase = nil
	}()

	tcproc := &testcase{"", nil, nil, make(chan error)}
	procpid, err := t.SpawnRegister("handler", factory_udp, gen.ProcessOptions{}, tcproc, true)
	if err != nil {
		t.Log().Error("unable to spawn 'handler' process: %s", err)
		t.testcase.err <- err
		return
	}
	t.Log().Info("spawned udp process with name: %s", procpid)

	tcnoproc := &testcase{"", nil, nil, make(chan error)}
	noprocpid, err := t.Spawn(factory_udp, gen.ProcessOptions{}, tcnoproc, false)
	if err != nil {
		t.Log().Error("unable to spawn udp process: %s", err)
		t.testcase.err <- err
		return
	}
	t.Log().Info("spawned udp process with no name and udp servers: %s", noprocpid)

	// test proc
	connproc, err := net.Dial("udp", "127.0.0.1:18181")
	defer connproc.Close()
	if _, err := connproc.Write([]byte("test proc")); err != nil {
		t.Log().Error("unable to send to 'proc': %s", err)
		t.testcase.err <- err
		return
	}
	if err := tcproc.wait(1); err != nil {
		t.Log().Error("udp with proc didn't receive udp-packet: %s", err)
		t.testcase.err <- err
		return
	}
	buf := make([]byte, 10)
	if n, err := connproc.Read(buf); err != nil {
		t.Log().Error("unable to read from udp socket: %s", err)
		t.testcase.err <- err
		return
	} else {
		data := buf[:n]
		if reflect.DeepEqual(data, []byte("proc")) == false {
			t.Log().Error("got incorrect data: %v (exp: 'proc')", data)
			t.testcase.err <- errIncorrect
			return
		}
	}

	// test noproc
	connnoproc, err := net.Dial("udp", "127.0.0.1:17171")
	defer connnoproc.Close()
	if _, err := connnoproc.Write([]byte("test no proc")); err != nil {
		t.Log().Error("unable to send to 'no proc': %s", err)
		t.testcase.err <- err
		return
	}
	if err := tcnoproc.wait(1); err != nil {
		t.Log().Error("udp with proc didn't receive udp-packet: %s", err)
		t.testcase.err <- err
		return
	}
	if n, err := connnoproc.Read(buf); err != nil {
		t.Log().Error("unable to read from udp socket: %s", err)
		t.testcase.err <- err
		return
	} else {
		data := buf[:n]
		if reflect.DeepEqual(data, []byte("noproc")) == false {
			t.Log().Error("got incorrect data: %v (exp: 'noproc')", data)
			t.testcase.err <- errIncorrect
			return
		}
	}

	t.testcase.err <- nil
}

func TestT17UDP(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	//nopt.Log.Level = gen.LogLevelTrace
	node, err := ergo.StartNode("t17node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t17, popt)
	if err != nil {
		panic(err)
	}

	t17cases = []*testcase{
		{"TestUDP", nil, nil, make(chan error)},
	}
	for _, tc := range t17cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(3); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
