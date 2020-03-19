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
	"time"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
)

var dTrace bool

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
	flag.BoolVar(&dTrace, "trace.dist", false, "trace erlang distribution protocol")
}

func dLog(f string, a ...interface{}) {
	if dTrace {
		log.Printf("d# "+f, a...)
	}
}

type flagId uint32

const (
	// distribution flags are defined here https://erlang.org/doc/apps/erts/erl_dist_protocol.html#distribution-flags
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
	SEND_SENDER                = 0x80000 // since OTP.21 enable replacement for SEND (distProtoSEND by distProtoSEND_SENDER)
	BIG_SEQTRACE_LABELS        = 0x100000
	EXIT_PAYLOAD               = 0x400000 // since OTP.22 enable replacement for EXIT, EXIT2, MONITOR_P_EXIT
	FRAGMENTS                  = 0x800000
)

type nodeFlag flagId

func (nf nodeFlag) toUint32() uint32 {
	return uint32(nf)
}

func (nf nodeFlag) isSet(f flagId) bool {
	return (uint32(nf) & uint32(f)) != 0
}

func toNodeFlag(f ...flagId) nodeFlag {
	var flags uint32
	for _, v := range f {
		flags |= uint32(v)
	}
	return nodeFlag(flags)
}

type Link struct {
	Name      string
	Cookie    string
	Hidden    bool
	peer      *Link
	conn      net.Conn
	challenge uint32
	flags     nodeFlag
	version   uint16
	term      *etf.Context
}

func Handshake(conn net.Conn, name, cookie string, hidden bool) (*Link, error) {

	link := &Link{
		Name:   name,
		Cookie: cookie,
		Hidden: hidden,

		flags: toNodeFlag(PUBLISHED, UNICODE_IO, DIST_MONITOR, DIST_MONITOR_NAME,
			EXTENDED_PIDS_PORTS, EXTENDED_REFERENCES,
			DIST_HDR_ATOM_CACHE, HIDDEN_ATOM_CACHE, NEW_FUN_TAGS,
			SMALL_ATOM_TAGS, UTF8_ATOMS, MAP_TAG, BIG_CREATION,
			FRAGMENTS,
		),

		conn: conn,

		version: 5,
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	link.composeName(b)
	if e := b.WriteTo(conn); e != nil {
		return nil, e
	}
	b.Reset()

	// define timeout for the handshaking
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	asyncReadChannel := make(chan error, 2)
	asyncRead := func() {
		//  If the buffer becomes too large, ReadFrom will panic with ErrTooLarge.
		defer func() {
			if r := recover(); r != nil {
				asyncReadChannel <- fmt.Errorf("malformed handshake (too large packet)")
			}
		}()
		_, e := b.ReadFrom(conn)
		asyncReadChannel <- e
	}
	for {
		go asyncRead()

		select {
		case <-timer.C:
			return nil, fmt.Errorf("timeout")

		case e := <-asyncReadChannel:
			if e != nil {
				return nil, e
			}
			switch b.B[0] {
			case 'n':
				// 'n' + 2 (version) + 4 (flags) + 4 (challenge) + name...
				if len(b.B) < 12 {
					return nil, fmt.Errorf("malformed handshake ('n')")
				}

				challenge := link.readChallenge(b.B)
				b.Reset()
				link.composeChallengeReply(challenge, b)
				if e := b.WriteTo(conn); e != nil {
					return nil, e
				}
				b.Reset()
				continue
			case 'a':
				// 'a' + 16 (digest)
				if len(b.B) != 17 {
					return nil, fmt.Errorf("malformed handshake ('a' length of digest)")
				}

				if !link.validateChallengeAck(b) {
					return nil, fmt.Errorf("malformed handshake ('a' digest)")
				}

				// handshaked
				return link, nil
			}

		}

	}

}

func HandshakeAccept(conn net.Conn, name, cookie string, hidden bool) (*Link, error) {
	link := &Link{
		Name:   name,
		Cookie: cookie,
		Hidden: hidden,

		flags: toNodeFlag(PUBLISHED, UNICODE_IO, DIST_MONITOR, DIST_MONITOR_NAME,
			EXTENDED_PIDS_PORTS, EXTENDED_REFERENCES,
			DIST_HDR_ATOM_CACHE, HIDDEN_ATOM_CACHE, NEW_FUN_TAGS,
			SMALL_ATOM_TAGS, UTF8_ATOMS, MAP_TAG, BIG_CREATION,
			FRAGMENTS,
		),

		challenge: rand.Uint32(),
		version:   5,
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	// define timeout for the handshaking
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	asyncReadChannel := make(chan error, 2)
	asyncRead := func() {
		//  If the buffer becomes too large, ReadFrom will panic with ErrTooLarge.
		defer func() {
			if r := recover(); r != nil {
				asyncReadChannel <- fmt.Errorf("malformed handshake (too large packet)")
			}
		}()
		n, e := b.ReadFrom(conn)
		fmt.Println("READ", n, e, b.B)
		asyncReadChannel <- e
	}
	for {
		go asyncRead()

		select {
		case <-timer.C:
			return nil, fmt.Errorf("timeout")
		case e := <-asyncReadChannel:
			if e != nil {
				return nil, e
			}

			switch b.B[2] {
			case 'n':
				if len(b.B) < 9 {
					return nil, fmt.Errorf("malformed handshake ('n' length)")
				}

				link.peer = link.readName(b.B[3:])
				b.Reset()
				link.composeStatus(b)
				if e := b.WriteTo(conn); e != nil {
					return nil, fmt.Errorf("malformed handshake ('n' accept name)")
				}

				b.Reset()
				link.composeChallenge(b)
				if e := b.WriteTo(conn); e != nil {
					return nil, e
				}
				b.Reset()
				continue
			case 'r':
				if len(b.B) < 21 {
					return nil, fmt.Errorf("malformed handshake ('r')")
				}

				if !link.validateChallengeReply(b.B[3:]) {
					return nil, fmt.Errorf("malformed handshake ('r1')")
				}
				b.Reset()

				link.composeChallengeAck(b)
				if e := b.WriteTo(conn); e != nil {
					return nil, e
				}
				b.Reset()
				// handshaked
				return link, nil

			default:
				fmt.Println("KKKK", b.B[3])
			}

		}

	}
}

func (l *Link) Close() {
	if l.conn != nil {
		l.conn.Close()
	}
}

func (l *Link) ReadMessage() (ts []etf.Term, err error) {

	sendData := func(headerLen int, data []byte) (int, error) {
		reply := make([]byte, len(data)+headerLen)
		if headerLen == 2 {
			binary.BigEndian.PutUint16(reply[0:headerLen], uint16(len(data)))
		} else {
			binary.BigEndian.PutUint32(reply[0:headerLen], uint32(len(data)))
		}
		copy(reply[headerLen:], data)
		dLog("Write to enode: %v", reply)
		return l.conn.Write(reply)
	}

	var length uint32
	var err1 error
	if err = binary.Read(l.conn, binary.BigEndian, &length); err != nil {
		return
	}
	if length == 0 {
		dLog("Keepalive (%s)", l.peer.Name)
		sendData(4, []byte{})
		return
	}
	r := &io.LimitedReader{
		R: l.conn,
		N: int64(length),
	}

	if l.flags.isSet(DIST_HDR_ATOM_CACHE) {
		var ctl, message etf.Term
		if err = l.readDist(r); err != nil {
			return //break
		}
		if ctl, err = l.readCtl(r); err != nil {
			return //break
		}
		dLog("READ CTL: %#v", ctl)

		if message, err1 = l.readMessage(r); err1 != nil {
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
				if res, err = l.readTerm(r); err != nil {
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

	return
}

func (l *Link) WriteMessage(ts []etf.Term) (err error) {
	sendData := func(data []byte) (int, error) {
		reply := make([]byte, len(data)+4)
		binary.BigEndian.PutUint32(reply[0:4], uint32(len(data)))
		copy(reply[4:], data)
		dLog("Write to enode: %v", reply)
		return l.conn.Write(reply)
	}

	buf := new(bytes.Buffer)
	if l.flags.isSet(DIST_HDR_ATOM_CACHE) {
		buf.Write([]byte{etf.EtVersion})
		l.term.WriteDist(buf, ts)
		for _, v := range ts {
			l.term.Write(buf, v)
		}
	} else {
		buf.Write([]byte{'p'})
		for _, v := range ts {
			buf.Write([]byte{etf.EtVersion})
			l.term.Write(buf, v)
		}
	}
	// dLog("WRITE: %#v: %#v", ts, buf.Bytes())
	_, err = sendData(buf.Bytes())
	return

}

func (l *Link) GetRemoteName() string {
	return l.peer.Name
}

func (l *Link) composeName(b *lib.Buffer) {
	dataLength := uint16(7 + len(l.Name)) // byte + uint16 + uint32 + len(l.Name)
	binary.BigEndian.PutUint16(b.B[0:2], dataLength)
	b.B[2] = 'n'
	binary.BigEndian.PutUint16(b.B[3:5], l.version)          // uint16
	binary.BigEndian.PutUint32(b.B[5:9], l.flags.toUint32()) // uint32
	b.Append([]byte(l.Name))
}

func (l *Link) readName(b []byte) *Link {
	peer := &Link{
		Name:    fmt.Sprintf("%s", b[6:]),
		version: binary.BigEndian.Uint16(b[0:2]),
		flags:   nodeFlag(binary.BigEndian.Uint32(b[2:6])),
	}
	return peer
}

func (l *Link) composeStatus(b *lib.Buffer) {
	//FIXME: there are few options for the status:
	// 	   ok, ok_simultaneous, nok, not_allowed, alive
	// More details here: https://erlang.org/doc/apps/erts/erl_dist_protocol.html#the-handshake-in-detail
	b.Allocate(2)
	dataLength := uint16(3) // 's' + "ok"
	binary.BigEndian.PutUint16(b.B[0:2], dataLength)
	b.Append([]byte("sok"))
}

func (l *Link) composeChallenge(b *lib.Buffer) {
	b.Allocate(13)
	dataLength := uint16(11 + len(l.Name))
	binary.BigEndian.PutUint16(b.B[0:2], dataLength)
	b.B[2] = 'n'
	binary.BigEndian.PutUint16(b.B[3:5], l.version)          // uint16
	binary.BigEndian.PutUint32(b.B[5:9], l.flags.toUint32()) // uint32
	binary.BigEndian.PutUint32(b.B[9:13], l.challenge)       // uint32
	b.Append([]byte(l.Name))
}

func (l *Link) readChallenge(msg []byte) (challenge uint32) {
	link := &Link{
		Name:    fmt.Sprintf("%s", msg[11:]),
		version: binary.BigEndian.Uint16(msg[1:3]),
		flags:   nodeFlag(binary.BigEndian.Uint32(msg[3:7])),
	}
	l.peer = link
	return binary.BigEndian.Uint32(msg[7:11])
}

func (l *Link) validateChallengeReply(b []byte) bool {
	l.peer.challenge = binary.BigEndian.Uint32(b[:4])
	digestB := b[4:]

	digestA := genDigest(l.challenge, l.Cookie)
	fmt.Println("DIGEST A:", digestA, l.peer.challenge, l.challenge)
	fmt.Println("DIGEST B:", digestB)
	return bytes.Equal(digestA[:], digestB)
}

func (l *Link) composeChallengeAck(b *lib.Buffer) {

	b.Allocate(3)
	dataLength := uint16(17) // 'a' + 16 (digest)
	binary.BigEndian.PutUint16(b.B[0:2], dataLength)
	b.B[2] = 'a'
	digest := genDigest(l.peer.challenge, l.Cookie)
	b.Append(digest[:])
}

func (l *Link) composeChallengeReply(challenge uint32, b *lib.Buffer) {
	digest := genDigest(challenge, l.Cookie)
	dataLength := uint16(21) // 1 (byte) + 4 (challenge) + 16 (digest)
	binary.BigEndian.PutUint16(b.B[0:2], dataLength)
	b.B[2] = 'r'
	binary.BigEndian.PutUint32(b.B[3:7], l.challenge) // uint32
	b.Append(digest[:])
}

func (l *Link) validateChallengeAck(b *lib.Buffer) bool {
	//digest := msg[1:]
	//FIXME
	return true
}

func genDigest(challenge uint32, cookie string) [16]byte {
	s := fmt.Sprintf("%s%d", cookie, challenge)
	return md5.Sum([]byte(s))
}

func (currentND *Link) readTerm(r io.Reader) (t etf.Term, err error) {
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

func (currentND *Link) readDist(r io.Reader) (err error) {
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

func (currentND *Link) readCtl(r io.Reader) (t etf.Term, err error) {
	t, err = currentND.term.Read(r)
	return
}

func (currentND *Link) readMessage(r io.Reader) (t etf.Term, err error) {
	t, err = currentND.term.Read(r)
	return
}
