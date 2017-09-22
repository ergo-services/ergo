package node

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

type mid uint8

const (
	EPMD_ALIVE2_REQ  = 120
	EPMD_ALIVE2_RESP = 121

	EPMD_PORT_PLEASE2_REQ = 122
	EPMD_PORT2_RESP       = 119

	// we don't need to support NAMES_REQ, DUMP_REQ and KILL_REQ... at least now
)

type EPMD struct {
	FullName string
	Name     string
	Domain   string

	// Listening port for incoming connections
	Port uint16

	// http://erlang.org/doc/reference_manual/distributed.html (section 13.5)
	// // 77 — regular public node, 72 — hidden
	Type uint8

	Protocol uint8
	HighVsn  uint16
	LowVsn   uint16
	Extra    []byte
	Creation uint16

	response chan interface{}
	out      chan []byte
}

func (e *EPMD) Init(name string, port uint16) {

	ns := strings.Split(name, "@")
	// TODO: add fqdn support

	e.FullName = name
	e.Name = ns[0]
	e.Domain = ns[1]
	e.Port = port
	e.Type = 77 // or 72 if hidden
	e.Protocol = 0
	e.HighVsn = 5
	e.LowVsn = 5
	e.Creation = 0

	conn, err := net.Dial("tcp", "127.0.0.1:4369")
	if err != nil {
		panic(err.Error())
	}

	in := make(chan []byte)
	go epmdREADER(conn, in)
	e.out = make(chan []byte)
	go epmdWRITER(conn, in, e.out)

	e.response = make(chan interface{})

	//epmd handler loop
	go func() {
		defer conn.Close()
		for {
			select {
			case reply := <-in:
				nLog("From EPMD: %v", reply)

				if len(reply) < 1 {
					continue
				}

				switch reply[0] {
				case EPMD_ALIVE2_RESP:
					e.response <- read_ALIVE2_RESP(reply)
				case EPMD_PORT2_RESP:
					e.response <- read_PORT2_RESP(reply)
				}
			}
		}
	}()

	e.register()

}

func (e *EPMD) register() {

	e.out <- compose_ALIVE2_REQ(e)
	creation := <-e.response

	switch creation {
	case false:
		panic(fmt.Sprintf("Duplicate name '%s'", e.Name))
	default:
		e.Creation = creation.(uint16)
	}
}

func (e *EPMD) ResolvePort(name string) uint16 {
	e.out <- compose_PORT_PLEASE2_REQ(name)
	value := <-e.response
	return value.(uint16)
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

func compose_ALIVE2_REQ(e *EPMD) (reply []byte) {
	reply = make([]byte, 14+len(e.Name)+len(e.Extra))
	reply[0] = byte(EPMD_ALIVE2_REQ)
	binary.BigEndian.PutUint16(reply[1:3], e.Port)
	reply[3] = e.Type
	reply[4] = e.Protocol
	binary.BigEndian.PutUint16(reply[5:7], e.HighVsn)
	binary.BigEndian.PutUint16(reply[7:9], e.LowVsn)
	nLen := len(e.Name)
	binary.BigEndian.PutUint16(reply[9:11], uint16(nLen))
	offset := (11 + nLen)
	copy(reply[11:offset], e.Name)
	nELen := len(e.Extra)
	binary.BigEndian.PutUint16(reply[offset:offset+2], uint16(nELen))
	copy(reply[offset+2:offset+2+nELen], e.Extra)
	return
}

func read_ALIVE2_RESP(reply []byte) interface{} {
	if reply[1] == 0 {
		return binary.BigEndian.Uint16(reply[2:4])
	}
	return false
}

func compose_PORT_PLEASE2_REQ(name string) (buf []byte) {
	buflen := uint16(len(name) + 1)
	buf = make([]byte, buflen)
	buf[0] = byte(EPMD_PORT_PLEASE2_REQ)
	copy(buf[1:buflen], name)
	return
}

func read_PORT2_RESP(reply []byte) (portno uint16) {
	fmt.Println("URA! %#v", reply)
	portno = binary.BigEndian.Uint16(reply[0:2])
	return
}
