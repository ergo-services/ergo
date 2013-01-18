package node

import (
	"encoding/binary"
	"erlang/dist"
	"erlang/epmd"
	"erlang/term"
	"flag"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
)

var nTrace bool

func init() {
	flag.BoolVar(&nTrace, "erlang.node.trace", false, "trace erlang node")
}

func nLog(f string, a ...interface{}) {
	if nTrace {
		log.Printf(f, a...)
	}
}

type Node struct {
	epmd.NodeInfo
	Cookie     string
	port       int32
	channels   map[term.Pid]procChannels
	registered map[term.Atom]term.Pid
}

type procChannels struct {
	in     chan term.Term
	inFrom chan term.Tuple
}

func NewNode(name string, cookie string) (node *Node) {
	nLog("Start with name '%s' and cookie '%s'", name, cookie)
	// TODO: add fqdn support
	ns := strings.Split(name, "@")
	nodeInfo := epmd.NodeInfo{
		FullName: name,
		Name:     ns[0],
		Domain:   ns[1],
		Port:     0,
		Type:     77, // or 72 if hidden
		Protocol: 0,
		HighVsn:  5,
		LowVsn:   5,
		Creation: 0,
	}

	node = &Node{
		NodeInfo:   nodeInfo,
		Cookie:     cookie,
		channels:   make(map[term.Pid]procChannels),
		registered: make(map[term.Atom]term.Pid),
	}
	return node
}

func (n *Node) prepareProcesses() {
	nkState := term.Tuple{}
	nkFun := func(nn *Node, msg term.Term, inState interface{}) (newState interface{}, tr *term.Term) {
		return net_kernel(nn, msg, inState)
	}
	nkPid := n.Spawn(nkFun, nkState)
	n.Register(term.Atom("net_kernel"), nkPid)

	gnsState := term.Tuple{}
	gnsFun := func(nn *Node, msg term.Term, inState interface{}) (newState interface{}, tr *term.Term) {
		return global_name_server(nn, msg, inState)
	}
	gnsPid := n.Spawn(gnsFun, gnsState)
	n.Register(term.Atom("global_name_server"), gnsPid)

}

func (n *Node) Spawn(lambda func(*Node, term.Term, interface{}) (interface{}, *term.Term), state interface{}) (pid term.Pid) {
	in := make(chan term.Term)
	inFrom := make(chan term.Tuple)
	pcs := procChannels{
		in:     in,
		inFrom: inFrom,
	}
	pid = n.storeProcess(pcs)
	go n.erlangProcess(pcs, lambda, state)
	in <- term.Tuple{term.Atom("go-ctl"), term.Tuple{term.Atom("your-pid"), pid}}
	return
}

func (n *Node) Register(name term.Atom, pid term.Pid) {
	n.registered[name] = pid
}

func (n *Node) Registered() (pids []term.Atom) {
	pids = make([]term.Atom, len(n.registered))
	i := 0
	for p, _ := range n.registered {
		pids[i] = p
		i++
	}
	return
}

func (n *Node) storeProcess(chs procChannels) (pid term.Pid) {
	pid = n.allocatePid()
	n.channels[pid] = chs
	return pid
}

func (n *Node) allocatePid() (pid term.Pid) {
	// FIXME: make proper allocation, now it just stub
	var id uint32 = 0
	for k, _ := range n.channels {
		if k.Id >= id {
			id = k.Id + 1
		}
	}
	pid.Node = term.Atom(n.FullName)
	pid.Id = id
	pid.Serial = 0 // FIXME
	pid.Creation = byte(n.Creation)
	return
}

func (n *Node) erlangProcess(pcs procChannels, lambda func(*Node, term.Term, interface{}) (interface{}, *term.Term), initState interface{}) {
	internalState := initState
	for {
		select {
		case msg := <-pcs.in:
			switch m := msg.(type) {
			case term.Tuple:
				switch mtag := m[0].(type) {
				case term.Atom:
					switch mtag {
					case term.Atom("go-ctl"):
						nLog("Control message: %#v", msg)
					default:
						internalState, _ = lambda(n, msg, internalState)
					}
				default:
					internalState, _ = lambda(n, msg, internalState)
				}
			default:
				internalState, _ = lambda(n, msg, internalState)
			}
		case msgFrom := <-pcs.inFrom:
			var reply *term.Term
			internalState, reply = lambda(n, msgFrom[1], internalState)
			if reply != nil {
				n.Send(msgFrom[0].(term.Pid), *reply)
			}
		}
	}
}

func (n *Node) Connect(remote string) {

}

func (n *Node) Publish(port int) (err error) {
	nLog("Publish ENode at %d", port)
	l, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(port)))
	if err != nil {
		return
	}
	n.Port = uint16(port)
	aliveResp := make(chan uint16)
	go epmdC(n, aliveResp)
	creation := <-aliveResp
	switch creation {
	case 99:
		return fmt.Errorf("Duplicate name '%s'", n.Name)
	case 100:
		return fmt.Errorf("Cannot connect to EPMD")
	default:
		n.Creation = creation
	}

	go func() {
		for {
			conn, err := l.Accept()
			nLog("Accept new at ENode")
			if err != nil {
				nLog(err.Error())
			} else {
				go n.mLoop(conn)
			}
		}
	}()
	n.prepareProcesses()
	return nil
}

func (currNode *Node) mLoop(c net.Conn) {

	currNd := dist.NewNodeDesc(currNode.FullName, currNode.Cookie, false)

	for {
		terms, err := currNd.ReadMessage(c)
		if err != nil {
			nLog("Enode error: %s", err.Error())
			break
		}
		currNode.handleTerms(c, terms)
	}
	c.Close()
}

func (currNode *Node) handleTerms(c net.Conn, terms []term.Term) {
	nLog("Node terms: %#v", terms)

	if len(terms) == 0 {
		return
	}
	switch t := terms[0].(type) {
	case term.Tuple:
		if len(t) > 0 {
			switch act := t.Element(1).(type) {
			case term.Int:
				switch act {
				case REG_SEND:
					if len(terms) == 2 {
						currNode.RegSend(t.Element(2), t.Element(4), terms[1])
					} else {
						nLog("*** ERROR: bad REG_SEND: %#v", terms)
					}
				default:
					nLog("Unhandled node message: %#v", t)
				}
			}
		}
	}
}

func (currNode *Node) RegSend(from, to term.Term, message term.Term) {
	nLog("REG_SEND: From: %#v, To: %#v, Message: %#v", from, to, message)
	var toPid term.Pid
	switch tp := to.(type) {
	case term.Pid:
		toPid = tp
	case term.Atom:
		toPid = currNode.Whereis(tp)
	}
	currNode.SendFrom(from, toPid, message)
}

func (currNode *Node) Whereis(who term.Atom) (pid term.Pid) {
	pid, _ = currNode.registered[who]
	return
}

func (currNode *Node) SendFrom(from term.Term, to term.Pid, message term.Term) {
	nLog("SendFrom: %#v, %#v, %#v", from, to, message)
	pcs := currNode.channels[to]
	pcs.inFrom <- term.Tuple{from, message}
}

func (currNode *Node) Send(to term.Pid, message term.Term) {
	nLog("Send: %#v, %#v", to, message)
	pcs := currNode.channels[to]
	pcs.in <- message
}

func epmdC(n *Node, resp chan uint16) {
	conn, err := net.Dial("tcp", ":4369")
	defer conn.Close()
	if err != nil {
		resp <- 100
		return
	}

	epmdFROM := make(chan []byte)
	go epmdREADER(conn, epmdFROM)

	epmdTO := make(chan []byte)
	go epmdWRITER(conn, epmdFROM, epmdTO)

	epmdTO <- epmd.Compose_ALIVE2_REQ(&n.NodeInfo)

	for {

		select {
		case reply := <-epmdFROM:
			nLog("From EPMD: %v", reply)

			switch epmd.MessageId(reply[0]) {
			case epmd.ALIVE2_RESP:
				if creation, ok := epmd.Read_ALIVE2_RESP(reply); ok {
					resp <- creation
				} else {
					resp <- 99
				}
			}

		}

	}
}

func epmdREADER(conn net.Conn, in chan []byte) {
	for {
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			in <- buf[0:n]
			in <- []byte{}
			return
		}
		nLog("Read from EPMD %d: %v", n, buf[:n])
		in <- buf[:n]
	}
}

func epmdWRITER(conn net.Conn, in chan []byte, out chan []byte) {
	for {
		select {
		case data := <-out:
			buf := make([]byte, 2)
			binary.BigEndian.PutUint16(buf[0:2], uint16(len(data)))
			buf = append(buf, data...)
			_, err := conn.Write(buf)
			if err != nil {
				in <- []byte{}
			}
		}
	}
}
