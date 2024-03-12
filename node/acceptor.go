package node

import (
	"net"

	"ergo.services/ergo/gen"
)

type acceptor struct {
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

func (a *acceptor) Cookie() string {
	return a.cookie
}

func (a *acceptor) SetCookie(cookie string) {
	a.cookie = cookie
}

func (a *acceptor) NetworkFlags() gen.NetworkFlags {
	return a.flags
}

func (a *acceptor) SetNetworkFlags(flags gen.NetworkFlags) {
	if flags.Enable == false {
		flags = gen.DefaultNetworkFlags
	}
	a.flags = flags
}

func (a *acceptor) MaxMessageSize() int {
	return a.maxmessagesize
}

func (a *acceptor) SetMaxMessageSize(size int) {
	if size < 0 {
		size = 0
	}
	a.maxmessagesize = size
}

func (a *acceptor) Info() gen.AcceptorInfo {
	return gen.AcceptorInfo{
		Interface:        a.l.Addr().String(),
		MaxMessageSize:   a.maxmessagesize,
		Flags:            a.flags,
		TLS:              a.tls,
		CustomRegistrar:  a.registrarCustom,
		RegistrarServer:  a.registrarServer,
		RegistrarVersion: a.registrarVersion,
		HandshakeVersion: a.handshake.Version(),
		ProtoVersion:     a.proto.Version(),
	}
}