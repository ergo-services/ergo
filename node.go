package node

import (
	"encoding/binary"
	"erlang/dist"
	"erlang/epmd"
	"flag"
	"fmt"
	erl "github.com/goerlang/etf/types"
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

type regReq struct {
	replyTo  chan erl.Pid
	channels procChannels
}

type registryChan struct {
	storeChan chan regReq
}

type nodeConn struct {
	conn  net.Conn
	wchan chan []erl.Term
}

type Node struct {
	epmd.NodeInfo
	Cookie     string
	port       int32
	registry   *registryChan
	channels   map[erl.Pid]procChannels
	registered map[erl.Atom]erl.Pid
	neighbors  map[erl.Atom]nodeConn
}

type procChannels struct {
	in     chan erl.Term
	inFrom chan erl.Tuple
	ctl    chan erl.Term
}

type Behaviour interface {
	ProcessLoop(n *Node, pid erl.Pid, pcs procChannels, pd Process, args ...interface{})
}

type Process interface {
	Behaviour() (behaviour Behaviour, options map[string]interface{})
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

	registry := &registryChan{
		storeChan: make(chan regReq),
	}

	node = &Node{
		NodeInfo:   nodeInfo,
		Cookie:     cookie,
		registry:   registry,
		channels:   make(map[erl.Pid]procChannels),
		registered: make(map[erl.Atom]erl.Pid),
		neighbors:  make(map[erl.Atom]nodeConn),
	}
	return node
}

func (n *Node) prepareProcesses() {
	nk := new(netKernel)
	nkPid := n.Spawn(nk, n)
	n.Register(erl.Atom("net_kernel"), nkPid)

	gns := new(globalNameServer)
	gnsPid := n.Spawn(gns, n)
	n.Register(erl.Atom("global_name_server"), gnsPid)

	rex := new(rexRPC)
	rexPid := n.Spawn(rex, n)
	n.Register(erl.Atom("rex"), rexPid)

}

func (n *Node) Spawn(pd Process, args ...interface{}) (pid erl.Pid) {
	behaviour, options := pd.Behaviour()
	chanSize, ok := options["chan-size"].(int)
	if !ok {
		chanSize = 100
	}
	ctlChanSize, ok := options["chan-size"].(int)
	if !ok {
		chanSize = 100
	}
	in := make(chan erl.Term, chanSize)
	inFrom := make(chan erl.Tuple, chanSize)
	ctl := make(chan erl.Term, ctlChanSize)
	pcs := procChannels{
		in:     in,
		inFrom: inFrom,
		ctl:    ctl,
	}
	pid = n.storeProcess(pcs)
	go behaviour.ProcessLoop(n, pid, pcs, pd, args...)
	return
}

func (n *Node) Register(name erl.Atom, pid erl.Pid) {
	n.registered[name] = pid
}

func (n *Node) Registered() (pids []erl.Atom) {
	pids = make([]erl.Atom, len(n.registered))
	i := 0
	for p, _ := range n.registered {
		pids[i] = p
		i++
	}
	return
}

func (n *Node) registrator() {
	for {
		select {
		case req := <-n.registry.storeChan:
			// FIXME: make proper allocation, now it just stub
			var id uint32 = 0
			for k, _ := range n.channels {
				if k.Id >= id {
					id = k.Id + 1
				}
			}
			var pid erl.Pid
			pid.Node = erl.Atom(n.FullName)
			pid.Id = id
			pid.Serial = 0 // FIXME
			pid.Creation = byte(n.Creation)

			n.channels[pid] = req.channels
			req.replyTo <- pid
		}
	}
}

func (n *Node) storeProcess(chs procChannels) (pid erl.Pid) {
	myChan := make(chan erl.Pid)
	n.registry.storeChan <- regReq{replyTo: myChan, channels: chs}
	pid = <-myChan
	return pid
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
				wchan := make(chan []erl.Term, 10)
				ndchan := make(chan *dist.NodeDesc)
				go n.mLoopReader(conn, wchan, ndchan)
				go n.mLoopWriter(conn, wchan, ndchan)
			}
		}
	}()
	go n.registrator()
	n.prepareProcesses()
	return nil
}

func (currNode *Node) mLoopReader(c net.Conn, wchan chan []erl.Term, ndchan chan *dist.NodeDesc) {

	currNd := dist.NewNodeDesc(currNode.FullName, currNode.Cookie, false)
	ndchan <- currNd
	for {
		terms, err := currNd.ReadMessage(c)
		if err != nil {
			nLog("Enode error: %s", err.Error())
			break
		}
		currNode.handleTerms(c, wchan, terms)
	}
	c.Close()
}

func (currNode *Node) mLoopWriter(c net.Conn, wchan chan []erl.Term, ndchan chan *dist.NodeDesc) {

	currNd := <-ndchan

	for {
		terms := <-wchan
		err := currNd.WriteMessage(c, terms)
		if err != nil {
			nLog("Enode error: %s", err.Error())
			break
		}
	}
	c.Close()
}

func (currNode *Node) handleTerms(c net.Conn, wchan chan []erl.Term, terms []erl.Term) {
	nLog("Node terms: %#v", terms)

	if len(terms) == 0 {
		return
	}
	switch t := terms[0].(type) {
	case erl.Tuple:
		if len(t) > 0 {
			switch act := t.Element(1).(type) {
			case int64:
				switch act {
				case int64(REG_SEND):
					if len(terms) == 2 {
						currNode.RegSend(t.Element(2), t.Element(4), terms[1])
					} else {
						nLog("*** ERROR: bad REG_SEND: %#v", terms)
					}
				default:
					nLog("Unhandled node message (act %d): %#v", act, t)
				}
			case erl.Atom:
				switch act {
				case erl.Atom("$go_set_node"):
					nLog("SET NODE %#v", t)
					currNode.neighbors[t[1].(erl.Atom)] = nodeConn{conn: c, wchan: wchan}
				}
			default:
				nLog("UNHANDLED ACT: %#v", t.Element(1))
			}
		}
	}
}

func (currNode *Node) RegSend(from, to erl.Term, message erl.Term) {
	nLog("REG_SEND: From: %#v, To: %#v, Message: %#v", from, to, message)
	var toPid erl.Pid
	switch tp := to.(type) {
	case erl.Pid:
		toPid = tp
	case erl.Atom:
		toPid = currNode.Whereis(tp)
	}
	currNode.SendFrom(from, toPid, message)
}

func (currNode *Node) Whereis(who erl.Atom) (pid erl.Pid) {
	pid, _ = currNode.registered[who]
	return
}

func (currNode *Node) SendFrom(from erl.Term, to erl.Pid, message erl.Term) {
	nLog("SendFrom: %#v, %#v, %#v", from, to, message)
	pcs := currNode.channels[to]
	pcs.inFrom <- erl.Tuple{from, message}
}

func (currNode *Node) Send(to erl.Pid, message erl.Term) {
	nLog("Send: %#v, %#v", to, message)
	if string(to.Node) == currNode.FullName {
		nLog("Send to local node")
		pcs := currNode.channels[to]
		pcs.in <- message
	} else {
		nLog("Send to remote node: %#v, %#v", to, currNode.neighbors[to.Node])

		msg := []erl.Term{erl.Tuple{SEND, erl.Atom(""), to}, message}
		currNode.neighbors[to.Node].wchan <- msg
	}
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
