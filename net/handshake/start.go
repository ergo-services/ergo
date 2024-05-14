package handshake

import (
	"crypto/sha256"
	"fmt"
	"net"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/edf"
)

func (h *handshake) Start(node gen.NodeHandshake, conn net.Conn, options gen.HandshakeOptions) (gen.HandshakeResult, error) {
	var result gen.HandshakeResult
	result.HandshakeVersion = h.Version()

	salt := lib.RandomString(64)
	hash := sha256.New()
	hash.Write([]byte(fmt.Sprintf("%s:%s", salt, options.Cookie)))
	digest := fmt.Sprintf("%x", hash.Sum(nil))

	hello := MessageHello{
		Salt:   salt,
		Digest: digest,
	}

	if err := h.writeMessage(conn, hello); err != nil {
		return result, err
	}

	v, tail, err := h.readMessage(conn, time.Second, nil)
	if err != nil {
		return result, err
	}

	hello2, ok := v.(MessageHello)
	if ok == false {
		return result, fmt.Errorf("malformed handshake Hello message")
	}
	hash = sha256.New()
	hash.Write([]byte(fmt.Sprintf("%s:%s:%s", hello2.Salt, hello.Digest, options.Cookie)))
	if hello2.Digest != fmt.Sprintf("%x", hash.Sum(nil)) {
		return result, fmt.Errorf("incorrect digest")
	}

	intro := MessageIntroduce{
		Node:     node.Name(),
		Version:  node.Version(),
		Flags:    options.Flags,
		Creation: node.Creation(),

		MaxMessageSize: options.MaxMessageSize,

		AtomCache: edf.GetAtomCache(),
		RegCache:  edf.GetRegCache(),
		ErrCache:  edf.GetErrCache(),
	}

	if err := h.writeMessage(conn, intro); err != nil {
		return result, err
	}

	// waiting for Accept message
	v, tail, err = h.readMessage(conn, time.Second, tail)
	if err != nil {
		return result, err
	}

	accept, ok := v.(MessageAccept)
	if ok == false {
		return result, fmt.Errorf("malformed handshake Accept message")
	}

	// waiting for Intro message
	v, tail, err = h.readMessage(conn, time.Second, tail)
	if err != nil {
		return result, err
	}

	intro2, ok := v.(MessageIntroduce)
	if ok == false {
		return result, fmt.Errorf("malformed handshake Introduce message")
	}

	if intro2.Node == node.Name() {
		return result, fmt.Errorf("malformed handshake Introduce message (same name)")
	}

	// everything looks good. just send an Accept message
	if err := h.writeMessage(conn, MessageAccept{}); err != nil {
		return result, err
	}

	result.ConnectionID = accept.ID
	result.Peer = intro2.Node
	result.PeerVersion = intro2.Version
	result.PeerCreation = intro2.Creation
	result.PeerFlags = intro2.Flags
	result.PeerMaxMessageSize = intro2.MaxMessageSize
	result.NodeFlags = options.Flags
	result.NodeMaxMessageSize = options.MaxMessageSize
	result.Tail = tail

	custom := ConnectionOptions{
		PoolSize:        accept.PoolSize,
		PoolDSN:         accept.PoolDSN,
		EncodeAtomCache: h.makeEncodeAtomCache(intro.AtomCache),
		EncodeRegCache:  h.makeEncodeRegCache(intro.RegCache),
		EncodeErrCache:  h.makeEncodeErrCache(intro.ErrCache),
		DecodeAtomCache: h.makeDecodeAtomCache(intro2.AtomCache),
		DecodeRegCache:  h.makeDecodeRegCache(intro2.RegCache),
		DecodeErrCache:  h.makeDecodeErrCache(intro.ErrCache, intro2.ErrCache),
	}
	result.Custom = custom

	if len(h.atom_mapping) > 0 {
		result.AtomMapping = make(map[gen.Atom]gen.Atom)
		for k, v := range h.atom_mapping {
			result.AtomMapping[k] = v
		}
	}

	return result, nil
}
