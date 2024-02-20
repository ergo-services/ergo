package node

import (
	"net"

	"ergo.services/ergo/gen"
)

type listener struct {
	l              net.Listener
	bs             int
	cookie         string
	port           uint16
	tls            bool
	flags          gen.NetworkFlags
	maxmessagesize int

	registrarCustom  bool
	registrarServer  string
	registrarVersion gen.Version

	handshake gen.NetworkHandshake
	proto     gen.NetworkProto
}

// gen.Acceptor interface implementation

func (l *listener) Cookie() string {
	return l.cookie
}

func (l *listener) SetCookie(cookie string) {
	l.cookie = cookie
}

func (l *listener) NetworkFlags() gen.NetworkFlags {
	return l.flags
}

func (l *listener) SetNetworkFlags(flags gen.NetworkFlags) {
	if flags.Enable == false {
		flags = gen.DefaultNetworkFlags
	}
	l.flags = flags
}

func (l *listener) MaxMessageSize() int {
	return l.maxmessagesize
}

func (l *listener) SetMaxMessageSize(size int) {
	if size < 0 {
		size = 0
	}
	l.maxmessagesize = size
}

func (l *listener) Info() gen.AcceptorInfo {
	return gen.AcceptorInfo{
		Interface:        l.l.Addr().String(),
		MaxMessageSize:   l.maxmessagesize,
		Flags:            l.flags,
		TLS:              l.tls,
		CustomRegistrar:  l.registrarCustom,
		RegistrarServer:  l.registrarServer,
		RegistrarVersion: l.registrarVersion,
		HandshakeVersion: l.handshake.Version(),
		ProtoVersion:     l.proto.Version(),
	}
}
