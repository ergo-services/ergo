package handshake

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/edf"
)

func (h *handshake) Negotiate(node gen.NodeHandshake, conn net.Conn, options gen.HandshakeOptions) (gen.HandshakeResult, error) {
	var result gen.HandshakeResult
	var salt string
	result.HandshakeVersion = h.Version()

	v, tail, err := h.readMessage(conn, time.Second, nil)
	if err != nil {
		return result, err
	}
	switch m := v.(type) {
	case MessageHello:
		hash := sha256.New()
		hash.Write([]byte(fmt.Sprintf("%s:%s", m.Salt, options.Cookie)))

		if m.Digest != fmt.Sprintf("%x", hash.Sum(nil)) {
			return result, fmt.Errorf("incorrect digest (accept stage 'hello')")
		}

		salt = lib.RandomString(64)
		hash = sha256.New()
		hash.Write([]byte(fmt.Sprintf("%s:%s:%s", salt, m.Digest, options.Cookie)))

		hello := MessageHello{
			Salt:   salt,
			Digest: fmt.Sprintf("%x", hash.Sum(nil)),
		}

		if fp := h.getLocalTLSFingerprint(conn, options.CertManager); fp != nil {
			hash = sha256.New()
			hash.Write([]byte(fmt.Sprintf("%s:%s:%s", salt, m.Salt, options.Cookie)))
			hash.Write(fp)
			hello.DigestCert = fmt.Sprintf("%x", hash.Sum(nil))
		}

		if err := h.writeMessage(conn, hello); err != nil {
			return result, err
		}

	case MessageJoin:
		result.Peer = m.Node
		hash := sha256.New()
		hash.Write([]byte(fmt.Sprintf("%s:%s:%s", m.ConnectionID, m.Salt, options.Cookie)))
		if m.Digest != fmt.Sprintf("%x", hash.Sum(nil)) {
			return result, fmt.Errorf("incorrect join digest")
		}
		result.ConnectionID = m.ConnectionID
		result.Custom = ConnectionOptions{}

		hash = sha256.New()
		hash.Write([]byte(fmt.Sprintf("%s:%s", m.Digest, options.Cookie)))
		accept := MessageAccept{
			Digest: fmt.Sprintf("%x", hash.Sum(nil)),
		}
		if fp := h.getLocalTLSFingerprint(conn, options.CertManager); fp != nil {
			hash = sha256.New()
			hash.Write([]byte(fmt.Sprintf("%s:%s:%s", m.Digest, m.Salt, options.Cookie)))
			hash.Write(fp)
			accept.DigestCert = fmt.Sprintf("%x", hash.Sum(nil))
		}
		if err := h.writeMessage(conn, accept); err != nil {
			return result, err
		}
		result.Tail = tail
		if len(h.atom_mapping) > 0 {
			result.AtomMapping = make(map[gen.Atom]gen.Atom)
			for k, v := range h.atom_mapping {
				result.AtomMapping[k] = v
			}
		}
		return result, nil

	default:
		return result, fmt.Errorf("malformed handshake Hello/Join message")
	}

	// wait for the introduce message
	v, tail, err = h.readMessage(conn, time.Second, nil)
	if err != nil {
		return result, err
	}

	intro, ok := v.(MessageIntroduce)
	if ok == false {
		return result, fmt.Errorf("malformed handshake Introduce message")
	}

	if intro.Node == node.Name() {
		return result, fmt.Errorf("malformed handshake Introduce message (same name)")
	}
	hash := sha256.New()
	hash.Write([]byte(fmt.Sprintf("%s:%s", salt, options.Cookie)))
	if intro.Digest != fmt.Sprintf("%x", hash.Sum(nil)) {
		return result, fmt.Errorf("incorrect digest (accept stage 'introduce')")
	}

	// deterministic connection ID (unconditional)
	connID := generateConnectionID(
		node.Name(), node.Creation(),
		intro.Node, intro.Creation,
		options.Cookie,
	)

	// Build the final accept and our introduce but do not send them yet: the node
	// registers the connection by ConnectionID first, then calls Accept to send these and
	// read the peer final accept. Registering before the introduce is sent means the peer
	// pool-join TCPs (it dials them only after reading our introduce) always find the
	// connection instead of racing registration under a connect storm.
	accept := MessageAccept{ID: connID, PoolSize: h.poolsize}
	accept.PoolDSN = append(accept.PoolDSN, conn.LocalAddr().String())

	intro2 := MessageIntroduce{
		Node:     node.Name(),
		Version:  node.Version(),
		Flags:    options.Flags,
		Creation: node.Creation(),

		MaxMessageSize: options.MaxMessageSize,

		AtomCache: edf.GetAtomCache(),
		RegCache:  edf.GetRegCache(),
		ErrCache:  edf.GetErrCache(),
	}

	result.ConnectionID = connID
	result.Peer = intro.Node
	result.PeerVersion = intro.Version
	result.PeerCreation = intro.Creation
	result.PeerFlags = intro.Flags
	result.PeerMaxMessageSize = intro.MaxMessageSize
	result.NodeFlags = options.Flags
	result.NodeMaxMessageSize = options.MaxMessageSize
	result.PoolSize = h.poolsize
	result.PoolDSN = accept.PoolDSN

	_, isTLS := conn.(*tls.Conn)
	result.Custom = ConnectionOptions{
		PoolSize:        h.poolsize,
		TLS:             isTLS,
		EncodeAtomCache: h.makeEncodeAtomCache(intro2.AtomCache),
		EncodeRegCache:  h.makeEncodeRegCache(intro2.RegCache),
		EncodeErrCache:  h.makeEncodeErrCache(intro2.ErrCache),
		DecodeAtomCache: h.makeDecodeAtomCache(intro.AtomCache),
		DecodeRegCache:  h.makeDecodeRegCache(intro.RegCache),
		DecodeErrCache:  h.makeDecodeErrCache(intro2.ErrCache, intro.ErrCache),

		pendingAccept:    accept,
		pendingIntroduce: intro2,
		pendingTail:      tail,
		awaitingAccept:   true,
	}

	return result, nil
}

func (h *handshake) getLocalTLSFingerprint(conn net.Conn, cm gen.CertManager) []byte {
	if _, tls := conn.(*tls.Conn); tls == false {
		return nil
	}
	cert := cm.GetCertificate()
	fp := sha1.Sum(cert.Certificate[0])
	return fp[:]
}

func generateConnectionID(nameA gen.Atom, creationA int64,
	nameB gen.Atom, creationB int64, cookie string) string {
	// canonical ordering: smaller name first
	first := fmt.Sprintf("%s:%d", nameA, creationA)
	second := fmt.Sprintf("%s:%d", nameB, creationB)
	if string(nameA) > string(nameB) {
		first, second = second, first
	}
	mac := hmac.New(sha256.New, []byte(cookie))
	mac.Write([]byte(first + ":" + second))
	return fmt.Sprintf("%x", mac.Sum(nil))
}
