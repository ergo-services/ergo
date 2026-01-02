package handshake

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

func (h *handshake) Join(node gen.NodeHandshake, conn net.Conn, id string, options gen.HandshakeOptions) ([]byte, error) {
	salt := lib.RandomString(64)
	hash := sha256.New()
	hash.Write([]byte(fmt.Sprintf("%s:%s:%s", id, salt, options.Cookie)))
	digest := fmt.Sprintf("%x", hash.Sum(nil))
	message := MessageJoin{
		Node:         node.Name(),
		ConnectionID: id,
		Salt:         salt,
		Digest:       digest,
	}

	if err := h.writeMessage(conn, message); err != nil {
		conn.Close()
		return nil, err
	}

	v, tail, err := h.readMessage(conn, time.Second, nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	accept, ok := v.(MessageAccept)
	if ok == false {
		return nil, fmt.Errorf("malformed handshake Accept message")
	}

	hash = sha256.New()
	hash.Write([]byte(fmt.Sprintf("%s:%s", message.Digest, options.Cookie)))
	if accept.Digest != fmt.Sprintf("%x", hash.Sum(nil)) {
		return nil, fmt.Errorf("incorrect digest (join)")
	}

	if fp := h.getRemoteTLSFingerprint(conn); fp != nil {
		hash = sha256.New()
		hash.Write([]byte(fmt.Sprintf("%s:%s:%s", digest, salt, options.Cookie)))
		hash.Write(fp)
		if accept.DigestCert != fmt.Sprintf("%x", hash.Sum(nil)) {
			return nil, fmt.Errorf("incorrect cert digest (join)")
		}
	}

	var status byte
	if len(tail) > 0 {
		status = tail[0]
		tail = tail[1:]
	} else {
		var b [1]byte
		conn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(conn, b[:]); err != nil {
			conn.Close()
			return nil, err
		}
		status = b[0]
	}

	switch status {
	case 0:
		return tail, nil
	case 1:
		conn.Close()
		return nil, fmt.Errorf("join rejected: connection id mismatch")
	case 2:
		conn.Close()
		return nil, fmt.Errorf("join rejected: no existing connection")
	default:
		conn.Close()
		return nil, fmt.Errorf("join rejected")
	}
}
