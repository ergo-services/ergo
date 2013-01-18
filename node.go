package node

import (
	"fmt"
	"encoding/binary"
	"erlang/epmd"
	"erlang/dist"
	"log"
	"net"
	"strconv"
	"strings"
)


type Node struct {
	epmd.NodeInfo
	Cookie string
	port int32
}

func NewNode(name string, cookie string) (n *Node){
	log.Printf("Start with name '%s' and cookie '%s'", name, cookie)
	// TODO: add fqdn support
	ns := strings.Split(name, "@")
	nodeInfo := epmd.NodeInfo{
		FullName: name,
		Name: ns[0],
		Domain: ns[1],
		Port: 0,
		Type: 77,				// or 72 if hidden
		Protocol: 0,
		HighVsn: 5,
		LowVsn: 5,
		Creation: 0,
	}

	return &Node{
		NodeInfo: nodeInfo,
		Cookie: cookie,
	}
}

func (n *Node) Connect(remote string) {

}

func (n *Node) Publish(port int) (err error) {
	log.Printf("Publish ENode at %d", port)
	l, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(port)))
	if err != nil {
		return
	}
	n.Port = uint16(port)
	aliveResp := make(chan uint16)
	go epmdC(n, aliveResp)
	creation := <- aliveResp
	switch creation {
	case 99:
		return fmt.Errorf("Duplicate name '%s'", n.Name)
	case 100:
		return fmt.Errorf("Cannot connect to EPMD")
	default:
		n.Creation = creation
	}

	go func () {
		for {
			conn, err := l.Accept()
			log.Printf("Accept new at ENode")
			if err != nil {
				log.Printf(err.Error())
			} else {
				go n.mLoop(conn)
			}
		}
	}()
	return nil
}


func (currNode *Node) mLoop(c net.Conn) {


	currNd := dist.NewNodeDesc(currNode.FullName, currNode.Cookie, false)

	for {
		err := currNd.ReadMessage(c)
		if err != nil {
			log.Printf("Enode error: %s", err.Error())
			break
		}


	}
	c.Close()
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
		case reply := <- epmdFROM:
			log.Printf("From EPMD: %v", reply)

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
		log.Printf("Read from EPMD %d: %v", n, buf[:n])
		in <- buf[:n]
	}
}


func epmdWRITER(conn net.Conn, in chan []byte, out chan []byte) {
	for {
		select {
		case data := <- out:
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
