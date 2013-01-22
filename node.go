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

type regReq struct {
	replyTo  chan term.Pid
	channels procChannels
}

type regNameReq struct {
	name term.Atom
	pid term.Pid
}

type unregNameReq struct {
	name term.Atom
}

type registryChan struct {
	storeChan chan regReq
	regNameChan chan regNameReq
	unregNameChan chan unregNameReq
}

type nodeConn struct {
	conn  net.Conn
	wchan chan []term.Term
}

type Node struct {
	epmd.NodeInfo
	Cookie     string
	port       int32
	registry   *registryChan
	channels   map[term.Pid]procChannels
	registered map[term.Atom]term.Pid
	neighbors  map[term.Atom]nodeConn
}

type procChannels struct {
	in     chan term.Term
	inFrom chan term.Tuple
	ctl    chan term.Term
}

type Behaviour interface {
	ProcessLoop(n *Node, pid term.Pid, pcs procChannels, pd Process, args ...interface{})
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
		regNameChan: make(chan regNameReq),
		unregNameChan: make(chan unregNameReq),
	}

	node = &Node{
		NodeInfo:   nodeInfo,
		Cookie:     cookie,
		registry:   registry,
		channels:   make(map[term.Pid]procChannels),
		registered: make(map[term.Atom]term.Pid),
		neighbors:  make(map[term.Atom]nodeConn),
	}
	return node
}

func (n *Node) prepareProcesses() {
	nk := new(netKernel)
	nkPid := n.Spawn(nk, n)
	n.Register(term.Atom("net_kernel"), nkPid)

	gns := new(globalNameServer)
	gnsPid := n.Spawn(gns, n)
	n.Register(term.Atom("global_name_server"), gnsPid)

	rex := new(rexRPC)
	rexPid := n.Spawn(rex, n)
	n.Register(term.Atom("rex"), rexPid)

}

func (n *Node) Spawn(pd Process, args ...interface{}) (pid term.Pid) {
	behaviour, options := pd.Behaviour()
	chanSize, ok := options["chan-size"].(int)
	if !ok {
		chanSize = 100
	}
	ctlChanSize, ok := options["chan-size"].(int)
	if !ok {
		chanSize = 100
	}
	in := make(chan term.Term, chanSize)
	inFrom := make(chan term.Tuple, chanSize)
	ctl := make(chan term.Term, ctlChanSize)
	pcs := procChannels{
		in:     in,
		inFrom: inFrom,
		ctl:    ctl,
	}
	pid = n.storeProcess(pcs)
	go behaviour.ProcessLoop(n, pid, pcs, pd, args...)
	return
}

func (n *Node) Register(name term.Atom, pid term.Pid) {
	r := regNameReq{name: name, pid: pid}
	n.registry.regNameChan <- r
}

func (n *Node) Unregister(name term.Atom) {
	r := unregNameReq{name: name}
	n.registry.unregNameChan <- r
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
			var pid term.Pid
			pid.Node = term.Atom(n.FullName)
			pid.Id = id
			pid.Serial = 0 // FIXME
			pid.Creation = byte(n.Creation)

			n.channels[pid] = req.channels
			req.replyTo <- pid
		case req := <-n.registry.regNameChan:
			n.registered[req.name] = req.pid
		case req := <-n.registry.unregNameChan:
			delete(n.registered, req.name)
		}
	}
}

func (n *Node) storeProcess(chs procChannels) (pid term.Pid) {
	myChan := make(chan term.Pid)
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
				wchan := make(chan []term.Term, 10)
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

func (currNode *Node) mLoopReader(c net.Conn, wchan chan []term.Term, ndchan chan *dist.NodeDesc) {

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

func (currNode *Node) mLoopWriter(c net.Conn, wchan chan []term.Term, ndchan chan *dist.NodeDesc) {

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

func (currNode *Node) handleTerms(c net.Conn, wchan chan []term.Term, terms []term.Term) {
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
			case term.Atom:
				switch act {
				case term.Atom("$go_set_node"):
					nLog("SET NODE %#v", t)
					currNode.neighbors[t[1].(term.Atom)] = nodeConn{conn: c, wchan: wchan}
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
	if string(to.Node) == currNode.FullName {
		nLog("Send to local node")
		pcs := currNode.channels[to]
		pcs.in <- message
	} else {
		nLog("Send to remote node: %#v, %#v", to, currNode.neighbors[to.Node])

		msg := []term.Term{term.Tuple{SEND, term.Atom(""), to}, message}
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
