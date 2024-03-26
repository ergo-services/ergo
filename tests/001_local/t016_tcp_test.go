package local

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/meta"
)

var (
	t16cases []*testcase
)

func factory_tcp() gen.ProcessBehavior {
	return &tcp{}
}

type tcp struct {
	act.Actor
	proc bool
	tc   *testcase

	addr net.Addr
}

func (tt *tcp) Init(args ...any) error {
	tt.tc = args[0].(*testcase)
	tt.proc = args[1].(bool)
	port := args[2].(uint16)
	if tt.proc {
		return nil
	}

	// TCP server with no Process
	opt := meta.TCPOptions{
		Port: port,
	}
	if metatcp, err := meta.CreateTCP(opt); err != nil {
		tt.Log().Error("unable to create tcp meta-process: %s", err)
		return nil
	} else {
		if id, err := tt.SpawnMeta(metatcp, gen.MetaOptions{}); err != nil {
			tt.Log().Error("unable to spawn tcp meta-process", err)
			metatcp.Terminate(err) // to stop TCP-listener
			return nil
		} else {
			tt.Log().Info("meta-process %s with tcp-server on %d started", id, port)
		}
	}

	// TCP server with Process
	opt = meta.TCPOptions{
		Port:        port + 1,
		ProcessPool: []gen.Atom{"handler1", "handler2"},
	}
	if metatcp, err := meta.CreateTCP(opt); err != nil {
		tt.Log().Error("unable to create tcp meta-process: %s", err)
		return nil
	} else {
		if id, err := tt.SpawnMeta(metatcp, gen.MetaOptions{}); err != nil {
			tt.Log().Error("unable to spawn tcp meta-process", err)
			metatcp.Terminate(err) // to stop TCP-listener
			return nil
		} else {
			tt.Log().Info("meta-process %s with tcp-server on %d started", id, port+1)
		}
	}
	return nil
}

func (tt *tcp) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case meta.MessageTCPConnect:
		tt.Log().Info("got new connection %s with %s", m.ID, m.RemoteAddr.String())
		tt.addr = m.RemoteAddr
		tt.tc.err <- nil
	case meta.MessageTCPDisconnect:
		tt.Log().Info("connection %s (%s) has terminated", m.ID, tt.addr.String())
		tt.tc.err <- nil
	case meta.MessageTCP:
		tt.Log().Info("server got tcp packet from %s: %q ", m.ID, string(m.Data))
		if tt.proc {
			m.Data = []byte(tt.Name())
		} else {
			m.Data = []byte("noproc")
		}
		if err := tt.SendAlias(m.ID, m); err != nil {
			tt.Log().Error("unable to send to %s: %s", m.ID, err)
		}
		tt.Log().Info("server sent reply")
		tt.tc.err <- nil
	default:
		tt.Log().Info("got unknown message from %s: %#v", from, message)
	}
	return nil
}

func factory_t16() gen.ProcessBehavior {
	return &t16{}
}

type t16 struct {
	act.Actor

	testcase *testcase
}

func (t *t16) HandleMessage(from gen.PID, message any) error {
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

func (t *t16) TestServer(input any) {
	defer func() {
		t.testcase = nil
	}()

	port := uint16(17171)
	tc_proc_handler1 := &testcase{"", nil, nil, make(chan error)}
	handler1_pid, err := t.SpawnRegister("handler1", factory_tcp, gen.ProcessOptions{}, tc_proc_handler1, true, port)
	if err != nil {
		t.Log().Error("unable to spawn 'handler1' process: %s", err)
		t.testcase.err <- err
		return
	}
	t.Log().Info("spawned handler1 process: %s", handler1_pid)

	tc_proc_handler2 := &testcase{"", nil, nil, make(chan error)}
	handler2_pid, err := t.SpawnRegister("handler2", factory_tcp, gen.ProcessOptions{}, tc_proc_handler2, true, port)
	if err != nil {
		t.Log().Error("unable to spawn 'handler2' process: %s", err)
		t.testcase.err <- err
		return
	}
	t.Log().Info("spawned handler2 process: %s", handler2_pid)

	tc_noproc := &testcase{"", nil, nil, make(chan error)}
	pid, err := t.Spawn(factory_tcp, gen.ProcessOptions{}, tc_noproc, false, port)
	if err != nil {
		t.Log().Error("unable to spawn process: %s", err)
		t.testcase.err <- err
		return
	}
	t.Log().Info("spawned process with tcp servers: %s", pid)

	// making tcp connect that must be handled by handler1
	addr1 := fmt.Sprintf("127.0.0.1:%d", port+1)
	conn_handler1, err := net.Dial("tcp", addr1)
	if err != nil {
		t.Log().Error("unable to connect to %s :%s", addr1, err)
		t.testcase.err <- err
		return
	}
	if err := tc_proc_handler1.wait(1); err != nil {
		t.Log().Error("handler 1 has no connection")
		t.testcase.err <- errIncorrect
		return
	}

	conn_handler1.Close()
	if err := tc_proc_handler1.wait(1); err != nil {
		t.Log().Error("handler 1 didnt receive meta.MessageTCPDissconnect")
		t.testcase.err <- errIncorrect
		return
	}

	// second try. must be handled by handler2
	conn_handler2, err := net.Dial("tcp", addr1)
	if err != nil {
		t.Log().Error("unable to connect to %s :%s", addr1, err)
		t.testcase.err <- err
		return
	}
	if err := tc_proc_handler2.wait(1); err != nil {
		t.Log().Error("handler 1 has no connection")
		t.testcase.err <- errIncorrect
		return
	}

	conn_handler2.Close()
	if err := tc_proc_handler2.wait(1); err != nil {
		t.Log().Error("handler 2 didnt receive meta.MessageTCPDissconnect")
		t.testcase.err <- errIncorrect
		return
	}

	// making tcp connect to 17171. must be handled by the process (that startd tcp-server) itself
	addr2 := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.Dial("tcp", addr2)
	if err != nil {
		t.Log().Error("unable to connect to %s :%s", addr2, err)
		t.testcase.err <- err
		return
	}
	if err := tc_noproc.wait(1); err != nil {
		t.Log().Error("handler noproc has no connection")
		t.testcase.err <- errIncorrect
		return
	}

	// test send/recv

	if _, err := conn.Write([]byte("hi")); err != nil {
		conn.Close()
		t.Log().Error("noproc unable to write: %s", err)
		t.testcase.err <- errIncorrect
		return
	}
	if err := tc_noproc.wait(1); err != nil {
		t.Log().Error("handler noproc didnt receive meta.MessageTCP")
		t.testcase.err <- errIncorrect
		return
	}

	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	buf := make([]byte, 10)
	if n, err := conn.Read(buf); err != nil {
		t.Log().Error("unable to read from tcp socket: %s", err)
		t.testcase.err <- err
		return
	} else {
		data := buf[:n]
		if reflect.DeepEqual(data, []byte("noproc")) == false {
			t.Log().Error("recv incorrect data: %v (exp: noproc)", string(data))
			t.testcase.err <- errIncorrect
			return
		}
	}

	conn.Close()
	if err := tc_noproc.wait(1); err != nil {
		t.Log().Error("handler noproc didnt receive meta.MessageTCPDissconnect")
		t.testcase.err <- errIncorrect
		return
	}

	t.testcase.err <- nil
}

func factory_tcpclient() gen.ProcessBehavior {
	return &tcpclient{}
}

type tcpclient struct {
	act.Actor

	id   gen.Alias
	tc   *testcase
	addr net.Addr
	port uint16
}

func (c *tcpclient) Init(args ...any) error {
	c.tc = args[0].(*testcase)
	c.port = args[1].(uint16)
	return nil
}

func (c *tcpclient) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case int:
		switch m {
		case 1:
			// create tcp conn
			opt := meta.TCPClientOptions{
				Port: c.port,
			}
			client, err := meta.CreateTCPClient(opt)
			if err != nil {
				c.Log().Error("unable to create tcp client: %s", err)
				c.tc.err <- err
				return nil
			}

			id, err := c.SpawnMeta(client, gen.MetaOptions{})
			if err != nil {
				c.Log().Error("unable to spawn tcp client meta process: %s", err)
				c.tc.err <- err
				return nil
			}
			c.id = id
			c.tc.err <- nil
			return nil

		case 2: // send a mess
			msg := meta.MessageTCP{
				Data: []byte("hi"),
			}
			if err := c.SendAlias(c.id, msg); err != nil {
				c.Log().Error("unable to send data: %s", err)
				c.tc.err <- err
				return nil
			}
			c.tc.err <- nil
			return nil

		case 3: // send exit to terminate conn
			if err := c.SendExitMeta(c.id, errors.New("whatever")); err != nil {
				c.Log().Error("unable to send exit signal: %s", err)
				c.tc.err <- err
				return nil
			}
			c.tc.err <- nil
			return nil
		}
		panic("unknown cmd")

	case meta.MessageTCPConnect:
		c.Log().Info("client. connection %s with %s", m.ID, m.RemoteAddr.String())
		c.addr = m.RemoteAddr
		c.tc.output = m
		c.tc.err <- nil
	case meta.MessageTCPDisconnect:
		c.tc.output = m
		c.Log().Info("client connection %s (%s) has terminated", m.ID, c.addr.String())
		c.tc.err <- nil
	case meta.MessageTCP:
		c.tc.output = m
		c.Log().Info("client got tcp packet from %s: %s ", m.ID, string(m.Data))
		c.tc.err <- nil
	default:
		c.Log().Info("got unknown message from %s: %#v", from, message)
	}
	return nil
}

func (t *t16) TestClient(input any) {
	defer func() {
		t.testcase = nil
	}()

	port := uint16(19191)
	tc_server := &testcase{"", nil, nil, make(chan error)}
	serverpid, err := t.Spawn(factory_tcp, gen.ProcessOptions{}, tc_server, false, port)
	if err != nil {
		t.Log().Error("unable to spawn process: %s", err)
		t.testcase.err <- err
		return
	}
	t.Log().Info("spawned process with tcp servers: %s", serverpid)

	tc_client := &testcase{"", nil, nil, make(chan error)}
	clientpid, err := t.Spawn(factory_tcpclient, gen.ProcessOptions{}, tc_client, port)
	if err != nil {
		t.Log().Error("unable to spawn process: %s", err)
		t.testcase.err <- err
		return
	}
	t.Log().Info("spawned process with tcp client: %s", clientpid)

	t.Send(clientpid, 1) // make tcp conn

	if err := tc_server.wait(1); err != nil {
		t.Log().Error("server proc should recv MessageTCPConnect. failed: %s", err)
		t.testcase.err <- err
		return
	}

	if err := tc_client.wait(1); err != nil {
		t.Log().Error("making tcp conn failed: %s", err)
		t.testcase.err <- err
		return
	}

	// client should recv MessageTCPConnect
	if err := tc_client.wait(int(1)); err != nil {
		t.Log().Error("didn't recv MessageTCPConnect: %s", err)
		t.testcase.err <- err
		return
	}

	if _, ok := tc_client.output.(meta.MessageTCPConnect); ok == false {
		t.Log().Error("incorrect value (exp: meta.MessageTCPConnect): %#v", tc_client.output)
		t.testcase.err <- errIncorrect
		return
	}

	t.Send(clientpid, 2) // ask to send a message
	if err := tc_client.wait(1); err != nil {
		t.Log().Error("ask to send a message failed: %s", err)
		t.testcase.err <- err
		return
	}

	// server should recv it and send reply
	if err := tc_server.wait(1); err != nil {
		t.Log().Error("server proc should recv MessageTCP. failed: %s", err)
		t.testcase.err <- err
		return
	}
	// client should recv MessageTCP
	if err := tc_client.wait(int(1)); err != nil {
		t.Log().Error("client didn't recv MessageTCP: %s", err)
		t.testcase.err <- err
		return
	}
	if _, ok := tc_client.output.(meta.MessageTCP); ok == false {
		t.Log().Error("incorrect value (exp: meta.MessageTCP): %#v", tc_client.output)
		t.testcase.err <- errIncorrect
		return
	}

	t.Send(clientpid, 3) // ask to terminate tcp conn

	if err := tc_server.wait(1); err != nil {
		t.Log().Error("server proc should recv MessageTCPDisconnect. failed: %s", err)
		t.testcase.err <- err
		return
	}

	if err := tc_client.wait(1); err != nil {
		t.Log().Error("term tcp conn failed: %s", err)
		t.testcase.err <- err
		return
	}

	// client should recv MessageTCPDisconnect
	if err := tc_client.wait(int(1)); err != nil {
		t.Log().Error("didn't recv MessageTCPDisonnect: %s", err)
		t.testcase.err <- err
		return
	}
	t.testcase.err <- nil
}

func TestT16TCP(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	// nopt.Log.Level = gen.LogLevelTrace
	node, err := ergo.StartNode("t16node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t16, popt)
	if err != nil {
		panic(err)
	}

	t16cases = []*testcase{
		{"TestServer", nil, nil, make(chan error)},
		{"TestClient", nil, nil, make(chan error)},
	}
	for _, tc := range t16cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(3); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
