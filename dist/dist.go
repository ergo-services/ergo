package dist

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/halturin/ergonode/etf"
)

var dTrace bool

func init() {
	flag.BoolVar(&dTrace, "trace.dist", false, "trace erlang distribution protocol")
}

func dLog(f string, a ...interface{}) {
	if dTrace {
		log.Printf("d# "+f, a...)
	}
}

type flagId uint32

const (
	PUBLISHED           flagId = 0x1
	ATOM_CACHE                 = 0x2
	EXTENDED_REFERENCES        = 0x4
	DIST_MONITOR               = 0x8
	FUN_TAGS                   = 0x10
	DIST_MONITOR_NAME          = 0x20
	HIDDEN_ATOM_CACHE          = 0x40
	NEW_FUN_TAGS               = 0x80
	EXTENDED_PIDS_PORTS        = 0x100
	EXPORT_PTR_TAG             = 0x200
	BIT_BINARIES               = 0x400
	NEW_FLOATS                 = 0x800
	UNICODE_IO                 = 0x1000
	DIST_HDR_ATOM_CACHE        = 0x2000
	SMALL_ATOM_TAGS            = 0x4000
	UTF8_ATOMS                 = 0x10000
	MAP_TAG                    = 0x20000
	BIG_CREATION               = 0x40000
)

type nodeFlag flagId

func (nf nodeFlag) toUint32() (flag uint32) {
	flag = uint32(nf)
	return
}

func (nf nodeFlag) isSet(f flagId) (is bool) {
	is = (uint32(nf) & uint32(f)) != 0
	return
}

func toNodeFlag(f ...flagId) (nf nodeFlag) {
	var flags uint32
	for _, v := range f {
		flags |= uint32(v)
	}
	nf = nodeFlag(flags)
	return
}

type nodeState uint8

const (
	HANDSHAKE nodeState = iota
	CONNECTED
)

type NodeDesc struct {
	Name       string
	Cookie     string
	Hidden     bool
	remote     *NodeDesc
	state      nodeState
	challenge  uint32
	flag       nodeFlag
	version    uint16
	term       *etf.Context
	isacceptor bool

	HandshakeError chan error
}

func NewNodeDesc(name, cookie string, isHidden bool, c net.Conn) (nd *NodeDesc) {
	nd = &NodeDesc{
		Name:   name,
		Cookie: cookie,
		Hidden: isHidden,
		remote: nil,
		state:  HANDSHAKE,
		flag: toNodeFlag(PUBLISHED, UNICODE_IO, DIST_MONITOR, DIST_MONITOR_NAME,
			EXTENDED_PIDS_PORTS, EXTENDED_REFERENCES,
			DIST_HDR_ATOM_CACHE, HIDDEN_ATOM_CACHE, NEW_FUN_TAGS,
			SMALL_ATOM_TAGS, UTF8_ATOMS, MAP_TAG, BIG_CREATION),
		version:        5,
		term:           new(etf.Context),
		isacceptor:     true,
		HandshakeError: make(chan error),
	}

	nd.term.ConvertBinaryToString = true

	// new connection. negotiate
	if c != nil {
		nd.isacceptor = false
		sn := nd.compose_SEND_NAME()
		negmessage := make([]byte, len(sn)+2)
		binary.BigEndian.PutUint16(negmessage[0:2], uint16(len(sn)))
		copy(negmessage[2:], sn)
		c.Write(negmessage)
	}

	return nd
}

func (currentND *NodeDesc) ReadMessage(c net.Conn) (ts []etf.Term, err error) {

	sendData := func(headerLen int, data []byte) (int, error) {
		reply := make([]byte, len(data)+headerLen)
		if headerLen == 2 {
			binary.BigEndian.PutUint16(reply[0:headerLen], uint16(len(data)))
		} else {
			binary.BigEndian.PutUint32(reply[0:headerLen], uint32(len(data)))
		}
		copy(reply[headerLen:], data)
		dLog("Write to enode: %v", reply)
		return c.Write(reply)
	}

	switch currentND.state {
	case HANDSHAKE:
		var length uint16
		if err = binary.Read(c, binary.BigEndian, &length); err != nil {
			currentND.HandshakeError <- err
			return
		}
		msg := make([]byte, length)
		if _, err = io.ReadFull(c, msg); err != nil {
			currentND.HandshakeError <- err
			return
		}
		dLog("Read from enode %d: %v", length, msg)

		switch msg[0] {
		case 'n':
			rand.Seed(time.Now().UTC().UnixNano())
			currentND.challenge = rand.Uint32()

			if currentND.isacceptor {
				sn := currentND.read_SEND_NAME(msg)
				// Statuses: ok, nok, ok_simultaneous, alive, not_allowed
				sok := currentND.compose_SEND_STATUS(sn, true)
				_, err = sendData(2, sok)
				if err != nil {
					currentND.HandshakeError <- err
					return
				}

				// Now send challenge
				challenge := currentND.compose_SEND_CHALLENGE(sn)
				sendData(2, challenge)
				if err != nil {
					currentND.HandshakeError <- err
					return
				}
			} else {
				//
				dLog("Doing CHALLENGE (outgoing connection)")

				challenge := currentND.read_SEND_CHALLENGE(msg)
				challengeReply := currentND.compose_SEND_CHALENGE_REPLY(challenge)
				sendData(2, challengeReply)
				return

			}

		case 'r':
			sn := currentND.remote
			ok := currentND.read_SEND_CHALLENGE_REPLY(sn, msg)
			if ok {
				challengeAck := currentND.compose_SEND_CHALLENGE_ACK(sn)
				sendData(2, challengeAck)
				if err != nil {
					currentND.HandshakeError <- err
					return
				}
				dLog("Remote: %#v", sn)
				ts = []etf.Term{etf.Term(etf.Tuple{etf.Atom("$connection"), etf.Atom(sn.Name), currentND.HandshakeError})}
			} else {
				err = fmt.Errorf("bad handshake")
				currentND.HandshakeError <- err
				return
			}
		case 's':
			r := string(msg[1:len(msg)])
			if r != "ok" {
				err = fmt.Errorf("Can't continue (recv_status: %s). Closing connection", r)
				currentND.HandshakeError <- err
			}

			return

		case 'a':
			currentND.read_SEND_CHALLENGE_ACK(msg)
			sn := currentND.remote
			dLog("Remote (outgoing): %#v", sn)
			ts = []etf.Term{etf.Term(etf.Tuple{etf.Atom("$connection"), etf.Atom(sn.Name), currentND.HandshakeError})}
			return
		}

	case CONNECTED:
		var length uint32
		var err1 error
		if err = binary.Read(c, binary.BigEndian, &length); err != nil {
			return
		}
		if length == 0 {
			dLog("Keepalive (%s)", currentND.remote.Name)
			sendData(4, []byte{})
			return
		}
		r := &io.LimitedReader{c, int64(length)}

		if currentND.flag.isSet(DIST_HDR_ATOM_CACHE) {
			var ctl, message etf.Term
			if err = currentND.readDist(r); err != nil {
				break
			}
			if ctl, err = currentND.readCtl(r); err != nil {
				break
			}
			dLog("READ CTL: %#v", ctl)

			if message, err1 = currentND.readMessage(r); err1 != nil {
				// break

			}
			dLog("READ MESSAGE: %#v", message)
			ts = append(ts, ctl, message)

		} else {
			msg := make([]byte, 1)
			if _, err = io.ReadFull(r, msg); err != nil {
				return
			}
			dLog("Read from enode %d: %#v", length, msg)

			switch msg[0] {
			case 'p':
				ts = make([]etf.Term, 0)
				for {
					var res etf.Term
					if res, err = currentND.readTerm(r); err != nil {
						break
					}
					ts = append(ts, res)
					dLog("READ TERM: %#v", res)
				}
				if err == io.EOF {
					err = nil
				}

			default:
				_, err = ioutil.ReadAll(r)
			}
		}

	}

	return
}

func (currentND *NodeDesc) WriteMessage(c net.Conn, ts []etf.Term) (err error) {
	sendData := func(data []byte) (int, error) {
		reply := make([]byte, len(data)+4)
		binary.BigEndian.PutUint32(reply[0:4], uint32(len(data)))
		copy(reply[4:], data)
		dLog("Write to enode: %v", reply)
		return c.Write(reply)
	}

	buf := new(bytes.Buffer)
	if currentND.flag.isSet(DIST_HDR_ATOM_CACHE) {
		buf.Write([]byte{etf.EtVersion})
		currentND.term.WriteDist(buf, ts)
		for _, v := range ts {
			currentND.term.Write(buf, v)
		}
	} else {
		buf.Write([]byte{'p'})
		for _, v := range ts {
			buf.Write([]byte{etf.EtVersion})
			currentND.term.Write(buf, v)
		}
	}
	// dLog("WRITE: %#v: %#v", ts, buf.Bytes())
	_, err = sendData(buf.Bytes())
	return

}

func (nd *NodeDesc) GetRemoteName() string {
	// nd.remote MUST not be nil otherwise is a bug. let it panic then
	if nd.state == CONNECTED {
		return nd.remote.Name
	}
	return ""
}

func (nd *NodeDesc) compose_SEND_NAME() (msg []byte) {
	msg = make([]byte, 7+len(nd.Name))
	msg[0] = byte('n')
	binary.BigEndian.PutUint16(msg[1:3], nd.version)
	binary.BigEndian.PutUint32(msg[3:7], nd.flag.toUint32())
	copy(msg[7:], nd.Name)
	return
}

func (currentND *NodeDesc) read_SEND_NAME(msg []byte) (nd *NodeDesc) {
	version := binary.BigEndian.Uint16(msg[1:3])
	flag := nodeFlag(binary.BigEndian.Uint32(msg[3:7]))
	name := string(msg[7:])
	nd = &NodeDesc{
		Name:    name,
		version: version,
		flag:    flag,
	}
	currentND.remote = nd
	return
}

func (currentND *NodeDesc) compose_SEND_STATUS(nd *NodeDesc, isOk bool) (msg []byte) {
	msg = make([]byte, 3)
	msg[0] = byte('s')
	copy(msg[1:], "ok")
	return
}

func (currentND *NodeDesc) compose_SEND_CHALLENGE(nd *NodeDesc) (msg []byte) {
	msg = make([]byte, 11+len(currentND.Name))
	msg[0] = byte('n')
	binary.BigEndian.PutUint16(msg[1:3], currentND.version)
	binary.BigEndian.PutUint32(msg[3:7], currentND.flag.toUint32())
	binary.BigEndian.PutUint32(msg[7:11], currentND.challenge)
	copy(msg[11:], currentND.Name)
	return
}

func (currentND *NodeDesc) read_SEND_CHALLENGE(msg []byte) (challenge uint32) {
	nd := &NodeDesc{
		Name:    string(msg[11:]),
		version: binary.BigEndian.Uint16(msg[1:3]),
		flag:    nodeFlag(binary.BigEndian.Uint32(msg[3:7])),
	}
	currentND.remote = nd
	return binary.BigEndian.Uint32(msg[7:11])
}

func (currentND *NodeDesc) read_SEND_CHALLENGE_REPLY(nd *NodeDesc, msg []byte) (isOk bool) {
	nd.challenge = binary.BigEndian.Uint32(msg[1:5])
	digestB := msg[5:]

	digestA := genDigest(currentND.challenge, currentND.Cookie)
	if bytes.Compare(digestA, digestB) == 0 {
		isOk = true
		currentND.state = CONNECTED
	} else {
		dLog("BAD HANDSHAKE: digestA: %+v, digestB: %+v", digestA, digestB)
		isOk = false
	}
	return
}

func (currentND *NodeDesc) compose_SEND_CHALLENGE_ACK(nd *NodeDesc) (msg []byte) {
	msg = make([]byte, 17)
	msg[0] = byte('a')

	digestB := genDigest(nd.challenge, currentND.Cookie) // FIXME: use his cookie, not mine

	copy(msg[1:], digestB)
	return
}

func (currentND *NodeDesc) compose_SEND_CHALENGE_REPLY(challenge uint32) (msg []byte) {
	msg = make([]byte, 21)
	msg[0] = byte('r')

	binary.BigEndian.PutUint32(msg[1:5], currentND.challenge)
	digest := genDigest(challenge, currentND.Cookie)
	copy(msg[5:], digest)
	return
}

func (currentND *NodeDesc) read_SEND_CHALLENGE_ACK(msg []byte) {
	currentND.state = CONNECTED
	return
}

func genDigest(challenge uint32, cookie string) (sum []byte) {
	h := md5.New()
	s := strings.Join([]string{cookie, strconv.FormatUint(uint64(challenge), 10)}, "")
	io.WriteString(h, s)
	sum = h.Sum(nil)
	return
}

func (nd NodeDesc) Flags() (flags []string) {
	fs := map[flagId]string{
		PUBLISHED:           "PUBLISHED",
		ATOM_CACHE:          "ATOM_CACHE",
		EXTENDED_REFERENCES: "EXTENDED_REFERENCES",
		DIST_MONITOR:        "DIST_MONITOR",
		FUN_TAGS:            "FUN_TAGS",
		DIST_MONITOR_NAME:   "DIST_MONITOR_NAME",
		HIDDEN_ATOM_CACHE:   "HIDDEN_ATOM_CACHE",
		NEW_FUN_TAGS:        "NEW_FUN_TAGS",
		EXTENDED_PIDS_PORTS: "EXTENDED_PIDS_PORTS",
		EXPORT_PTR_TAG:      "EXPORT_PTR_TAG",
		BIT_BINARIES:        "BIT_BINARIES",
		NEW_FLOATS:          "NEW_FLOATS",
		UNICODE_IO:          "UNICODE_IO",
		DIST_HDR_ATOM_CACHE: "DIST_HDR_ATOM_CACHE",
		SMALL_ATOM_TAGS:     "SMALL_ATOM_TAGS",
		UTF8_ATOMS:          "UTF8_ATOMS",
		MAP_TAG:             "MAP_TAG",
		BIG_CREATION:        "BIG_CREATION",
	}

	for k, v := range fs {
		if nd.flag.isSet(k) {
			flags = append(flags, v)
		}
	}
	return
}

func (currentND *NodeDesc) readTerm(r io.Reader) (t etf.Term, err error) {
	b := make([]byte, 1)
	_, err = io.ReadFull(r, b)

	if err != nil {
		return
	}
	if b[0] != etf.EtVersion {
		err = fmt.Errorf("Not ETF: %d", b[0])
		return
	}
	t, err = currentND.term.Read(r)
	return
}

func (currentND *NodeDesc) readDist(r io.Reader) (err error) {
	b := make([]byte, 1)
	_, err = io.ReadFull(r, b)

	if err != nil {
		return
	}
	if b[0] != etf.EtVersion {
		err = fmt.Errorf("Not dist header: %d", b[0])
		return
	}
	return currentND.term.ReadDist(r)
}

func (currentND *NodeDesc) readCtl(r io.Reader) (t etf.Term, err error) {
	t, err = currentND.term.Read(r)
	return
}

func (currentND *NodeDesc) readMessage(r io.Reader) (t etf.Term, err error) {
	t, err = currentND.term.Read(r)
	return
}
