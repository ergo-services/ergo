package cloud

import (
	"crypto/md5"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
)

const (
	EventCloud gen.Event = "cloud"

	ProtoHandshakeV1                = 41
	ProtoHandshakeV1Auth            = 100
	ProtoHandshakeV1AuthReply       = 101
	ProtoHandshakeV1Challenge       = 102
	ProtoHandshakeV1ChallengeAccept = 103
	ProtoHandshakeV1Error           = 200
)

type MessageEventCloud struct {
	Online bool
}

func RegisterTypes() error {
	types := []interface{}{
		node.CloudFlags{},
		MessageHandshakeV1Auth{},
		MessageHandshakeV1AuthReply{},
		MessageHandshakeV1Challenge{},
		MessageHandshakeV1ChallengeAccept{},
		MessageHandshakeV1Error{},
	}
	rtOpts := etf.RegisterTypeOptions{Strict: true}

	for _, t := range types {
		if _, err := etf.RegisterType(t, rtOpts); err != nil && err != lib.ErrTaken {
			return err
		}
	}
	return nil
}

func GenDigest(a, b, c string) [16]byte {
	digest := md5.Sum([]byte(a + b + c))
	return digest
}

// client -> cloud
type MessageHandshakeV1Auth struct {
	Node     string
	Cluster  string
	Creation uint32
	Flags    node.CloudFlags
}

// cloud -> client
type MessageHandshakeV1AuthReply struct {
	Node     string
	Creation uint32
	Digest   string
}

// client -> cloud
type MessageHandshakeV1Challenge struct {
	Digest string
}

// cloud -> client
type MessageHandshakeV1ChallengeAccept struct {
	Node string // mapped node name
}

// cloud -> client
type MessageHandshakeV1Error struct {
	Reason string
}
