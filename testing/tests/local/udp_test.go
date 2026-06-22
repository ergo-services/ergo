package local

import (
	"net"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/meta"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// udpHandler is the target process of a UDP server with Process set: it reports
// the received payload to the collector (a meta delivering to its own parent is a
// self-send that bypasses the core recorder, so the receiver re-reports), and
// replies "proc" back over the socket.
type udpHandler struct {
	act.Actor
	collector gen.PID
	reply     string
}

func factoryUdpHandler() gen.ProcessBehavior { return &udpHandler{reply: "proc"} }

func (h *udpHandler) Init(args ...any) error {
	h.collector = args[0].(gen.PID)
	return nil
}

func (h *udpHandler) HandleMessage(from gen.PID, message any) error {
	if m, ok := message.(meta.MessageUDP); ok {
		h.Send(h.collector, string(m.Data))
		m.Data = []byte(h.reply)
		return h.SendAlias(m.ID, m)
	}
	return nil
}

// udpOwner spawns two UDP servers on ephemeral ports: one with no Process (its
// packets are delivered to the owner) and one with Process "handler". It reports
// payloads delivered to itself and replies "noproc".
type udpOwner struct {
	act.Actor
	collector    gen.PID
	serverProc   gen.Alias
	serverNoProc gen.Alias
}

func factoryUdpOwner() gen.ProcessBehavior { return &udpOwner{} }

func (o *udpOwner) Init(args ...any) error {
	o.collector = args[0].(gen.PID)

	s1, err := meta.CreateUDPServer(meta.UDPServerOptions{Host: "127.0.0.1", Port: 0})
	if err != nil {
		return err
	}
	if o.serverNoProc, err = o.SpawnMeta(s1, gen.MetaOptions{}); err != nil {
		return err
	}

	s2, err := meta.CreateUDPServer(meta.UDPServerOptions{Host: "127.0.0.1", Port: 0, Process: "handler"})
	if err != nil {
		return err
	}
	if o.serverProc, err = o.SpawnMeta(s2, gen.MetaOptions{}); err != nil {
		return err
	}
	return nil
}

func (o *udpOwner) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request {
	case "addr_proc":
		insp, err := o.InspectMeta(o.serverProc)
		if err != nil {
			return err, nil
		}
		return insp["listener"], nil
	case "addr_noproc":
		insp, err := o.InspectMeta(o.serverNoProc)
		if err != nil {
			return err, nil
		}
		return insp["listener"], nil
	}
	return "ok", nil
}

func (o *udpOwner) HandleMessage(from gen.PID, message any) error {
	if m, ok := message.(meta.MessageUDP); ok {
		o.Send(o.collector, string(m.Data))
		m.Data = []byte("noproc")
		return o.SendAlias(m.ID, m)
	}
	return nil
}

func udpReadReply(t *testing.T, conn net.Conn) string {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 16)
	nr, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("udp read: %s", err)
	}
	return string(buf[:nr])
}

// TestLocalUDP: a UDP server with Process delivers datagrams to that process
// (which replies "proc"); a server with no Process delivers to its owner (which
// replies "noproc"). Each reply is read back over the socket.
func TestLocalUDP(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")

	collector := n.Spawn(factoryEcho, gen.ProcessOptions{})
	handler := n.SpawnRegister("handler", factoryUdpHandler, gen.ProcessOptions{}, collector)
	owner := n.Spawn(factoryUdpOwner, gen.ProcessOptions{}, collector)

	addrProcAny, err := n.Call(owner, "addr_proc")
	check.NoError(t, err)
	addrProc := addrProcAny.(string)
	addrNoProcAny, err := n.Call(owner, "addr_noproc")
	check.NoError(t, err)
	addrNoProc := addrNoProcAny.(string)

	// Process server -> handler, replies "proc"
	connP, err := net.Dial("udp", addrProc)
	check.NoError(t, err)
	defer connP.Close()
	mk := n.Mark()
	_, err = connP.Write([]byte("test proc"))
	check.NoError(t, err)
	n.ShouldSend().From(handler).Message("test proc").Since(mk).Once().Within(time.Second).Must()
	check.Equal(t, "proc", udpReadReply(t, connP))

	// no-Process server -> owner, replies "noproc"
	connN, err := net.Dial("udp", addrNoProc)
	check.NoError(t, err)
	defer connN.Close()
	mk = n.Mark()
	_, err = connN.Write([]byte("test no proc"))
	check.NoError(t, err)
	n.ShouldSend().From(owner).Message("test no proc").Since(mk).Once().Within(time.Second).Must()
	check.Equal(t, "noproc", udpReadReply(t, connN))
}
