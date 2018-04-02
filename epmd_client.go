package ergonode

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
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

	// Listening gen_server port for incoming connections
	Port uint16

	// Listening empd daemon port. This field allows support for custom epmd ports,
	// and potentially multiple discrete node clusters.
	EPMDPort uint16

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

// Init attempts to connect to the local epmd daemon and supply it the server
// configuration. You must have an epmd daemon running on the same machine that is
// running the ergonode-powered process. You cannot use an epmd daemon on a remote
// machine. If nodeArgs are not supplied, Init assumes that the epmd daemon is
// listening on loopback ip 127.0.0.1 and port 4369.
// If present, the custom nodeArgs must be of type *EPMDArgs (see NewEPMDArgs)
func (e *EPMD) Init(name string, port uint16, nodeArgs ...NodeArgs) {

	var epmdPort uint16 = 4369
	var epmdHost = "127.0.0.1"

	// Attempt to find custom EPMD args
	if len(nodeArgs) > 0 {
	epmdLoop:
		for i := range nodeArgs {
			switch t := nodeArgs[i].(type) {
			case *EPMDArgs:
				epmdPort = t.Port
				epmdHost = t.Host
				break epmdLoop
			}
		}
	}

	ns := strings.Split(name, "@")
	// TODO: add fqdn support

	e.FullName = name
	e.Name = ns[0]
	e.Domain = ns[1]
	e.Port = port
	e.EPMDPort = epmdPort
	e.Type = 77 // or 72 if hidden
	e.Protocol = 0
	e.HighVsn = 5
	e.LowVsn = 5
	e.Creation = 0

	conn, err := net.Dial("tcp", net.JoinHostPort(epmdHost, strconv.Itoa(int(epmdPort))))
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

func (e *EPMD) ResolvePort(name string) int {
	var err error
	ns := strings.Split(name, "@")

	conn, err := net.Dial("tcp", net.JoinHostPort(ns[1], strconv.Itoa(int(e.EPMDPort))))
	if err != nil {
		return -1
	}

	defer conn.Close()

	data := compose_PORT_PLEASE2_REQ(ns[0])
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf[0:2], uint16(len(data)))
	buf = append(buf, data...)
	_, err = conn.Write(buf)
	if err != nil {
		return -1
	}

	buf = make([]byte, 1024)
	_, err = conn.Read(buf)
	if err != nil {
		return -1
	}

	value := read_PORT2_RESP(buf)

	return int(value)
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

func read_PORT2_RESP(reply []byte) (portno int) {
	if reply[0] == 119 && reply[1] == 0 {
		p := binary.BigEndian.Uint16(reply[2:4])
		portno = int(p)
	} else {
		portno = -1
	}
	return
}
