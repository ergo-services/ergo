package cloud

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"io"
	"net"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
)

const (
	defaultHandshakeTimeout = 5 * time.Second
	clusterNameLengthMax    = 128
)

type Handshake struct {
	node.Handshake
	nodename string
	creation uint32
	options  node.Cloud
	flags    node.Flags
}

type handshakeDetails struct {
	cookieHash   []byte
	digestRemote []byte
	details      node.HandshakeDetails
	mapName      string
	hash         hash.Hash
}

func createHandshake(options node.Cloud) (node.HandshakeInterface, error) {
	if options.Timeout == 0 {
		options.Timeout = defaultHandshakeTimeout
	}

	if err := RegisterTypes(); err != nil {
		return nil, err
	}

	return &Handshake{
		options: options,
	}, nil
}

func (ch *Handshake) Init(nodename string, creation uint32, flags node.Flags) error {
	if flags.EnableProxy == false {
		s := "proxy feature must be enabled for the cloud connection"
		lib.Warning(s)
		return fmt.Errorf(s)
	}
	if ch.options.Cluster == "" {
		s := "option Cloud.Cluster can not be empty"
		lib.Warning(s)
		return fmt.Errorf(s)
	}
	if len(ch.options.Cluster) > clusterNameLengthMax {
		s := "option Cloud.Cluster has too long name"
		lib.Warning(s)
		return fmt.Errorf(s)
	}
	ch.nodename = nodename
	ch.creation = creation
	ch.flags = flags
	if ch.options.Flags.Enable == false {
		return nil
	}

	ch.flags.EnableRemoteSpawn = ch.options.Flags.EnableRemoteSpawn
	return nil
}

func (ch *Handshake) Start(remote net.Addr, conn lib.NetReadWriter, tls bool, cookie string) (node.HandshakeDetails, error) {
	hash := sha256.New()
	handshake := &handshakeDetails{
		cookieHash: hash.Sum([]byte(cookie)),
		hash:       hash,
	}
	handshake.details.Flags = ch.flags

	ch.sendV1Auth(conn)

	// define timeout for the handshaking
	timer := time.NewTimer(ch.options.Timeout)
	defer timer.Stop()

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	asyncReadChannel := make(chan error, 2)
	asyncRead := func() {
		_, err := b.ReadDataFrom(conn, 1024)
		asyncReadChannel <- err
	}

	expectingBytes := 4
	await := []byte{ProtoHandshakeV1AuthReply, ProtoHandshakeV1Error}

	for {
		go asyncRead()
		select {
		case <-timer.C:
			return handshake.details, fmt.Errorf("timeout")
		case err := <-asyncReadChannel:
			if err != nil {
				return handshake.details, err
			}

			if b.Len() < expectingBytes {
				continue
			}

			if b.B[0] != ProtoHandshakeV1 {
				return handshake.details, fmt.Errorf("malformed handshake proto")
			}

			l := int(binary.BigEndian.Uint16(b.B[2:4]))
			buffer := b.B[4 : l+4]

			if len(buffer) != l {
				return handshake.details, fmt.Errorf("malformed handshake (wrong packet length)")
			}

			// check if we got correct message type regarding to 'await' value
			if bytes.Count(await, b.B[1:2]) == 0 {
				return handshake.details, fmt.Errorf("malformed handshake sequence")
			}

			await, err = ch.handle(conn, b.B[1], buffer, handshake)
			if err != nil {
				return handshake.details, err
			}

			b.Reset()
		}

		if await == nil {
			// handshaked
			break
		}
	}

	return handshake.details, nil
}

func (ch *Handshake) handle(socket io.Writer, messageType byte, buffer []byte, details *handshakeDetails) ([]byte, error) {
	switch messageType {
	case ProtoHandshakeV1AuthReply:
		if err := ch.handleV1AuthReply(buffer, details); err != nil {
			return nil, err
		}
		if err := ch.sendV1Challenge(socket, details); err != nil {
			return nil, err
		}
		return []byte{ProtoHandshakeV1ChallengeAccept, ProtoHandshakeV1Error}, nil

	case ProtoHandshakeV1ChallengeAccept:
		if err := ch.handleV1ChallegeAccept(buffer, details); err != nil {
			return nil, err
		}
		return nil, nil

	case ProtoHandshakeV1Error:
		return nil, ch.handleV1Error(buffer)

	default:
		return nil, fmt.Errorf("unknown message type")
	}
}

func (ch *Handshake) sendV1Auth(socket io.Writer) error {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	message := MessageHandshakeV1Auth{
		Node:     ch.nodename,
		Cluster:  ch.options.Cluster,
		Creation: ch.creation,
		Flags:    ch.options.Flags,
	}
	b.Allocate(1 + 1 + 2)
	b.B[0] = ProtoHandshakeV1
	b.B[1] = ProtoHandshakeV1Auth
	if err := etf.Encode(message, b, etf.EncodeOptions{}); err != nil {
		return err
	}
	binary.BigEndian.PutUint16(b.B[2:4], uint16(b.Len()-4))
	if err := b.WriteDataTo(socket); err != nil {
		return err
	}

	return nil
}

func (ch *Handshake) sendV1Challenge(socket io.Writer, handshake *handshakeDetails) error {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	digest := GenDigest(handshake.hash, []byte(ch.nodename), handshake.digestRemote, handshake.cookieHash)
	message := MessageHandshakeV1Challenge{
		Digest: digest,
	}
	b.Allocate(1 + 1 + 2)
	b.B[0] = ProtoHandshakeV1
	b.B[1] = ProtoHandshakeV1Challenge
	if err := etf.Encode(message, b, etf.EncodeOptions{}); err != nil {
		return err
	}
	binary.BigEndian.PutUint16(b.B[2:4], uint16(b.Len()-4))
	if err := b.WriteDataTo(socket); err != nil {
		return err
	}

	return nil

}

func (ch *Handshake) handleV1AuthReply(buffer []byte, handshake *handshakeDetails) error {
	m, _, err := etf.Decode(buffer, nil, etf.DecodeOptions{})
	if err != nil {
		return fmt.Errorf("malformed MessageHandshakeV1AuthReply message: %s", err)
	}
	message, ok := m.(MessageHandshakeV1AuthReply)
	if ok == false {
		return fmt.Errorf("malformed MessageHandshakeV1AuthReply message: %#v", m)
	}

	digest := GenDigest(handshake.hash, []byte(message.Node), []byte(ch.options.Cluster), handshake.cookieHash)
	if bytes.Compare(message.Digest, digest) != 0 {
		return fmt.Errorf("wrong digest")
	}
	handshake.digestRemote = digest
	handshake.details.Name = message.Node
	handshake.details.Creation = message.Creation

	return nil
}

func (ch *Handshake) handleV1ChallegeAccept(buffer []byte, handshake *handshakeDetails) error {
	m, _, err := etf.Decode(buffer, nil, etf.DecodeOptions{})
	if err != nil {
		return fmt.Errorf("malformed MessageHandshakeV1ChallengeAccept message: %s", err)
	}
	message, ok := m.(MessageHandshakeV1ChallengeAccept)
	if ok == false {
		return fmt.Errorf("malformed MessageHandshakeV1ChallengeAccept message: %#v", m)
	}

	mapping := etf.NewAtomMapping()
	mapping.In[etf.Atom(message.Node)] = etf.Atom(ch.nodename)
	mapping.Out[etf.Atom(ch.nodename)] = etf.Atom(message.Node)
	handshake.details.AtomMapping = mapping
	handshake.mapName = message.Node
	return nil
}

func (ch *Handshake) handleV1Error(buffer []byte) error {
	m, _, err := etf.Decode(buffer, nil, etf.DecodeOptions{})
	if err != nil {
		return fmt.Errorf("malformed MessageHandshakeV1Error message: %s", err)
	}
	message, ok := m.(MessageHandshakeV1Error)
	if ok == false {
		return fmt.Errorf("malformed MessageHandshakeV1Error message: %#v", m)
	}
	return fmt.Errorf(message.Reason)
}
