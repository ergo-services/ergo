package node

import (
	"net"

	"ergo.services/ergo/gen"
)

type acceptor struct {
	l                net.Listener
	bs               int
	cookie           string
	port             uint16
	cert_manager     gen.CertManager
	flags            gen.NetworkFlags
	max_message_size int

	registrar_custom bool
	registrar_info   func() gen.RegistrarInfo

	handshake gen.NetworkHandshake
	proto     gen.NetworkProto

	atom_mapping map[gen.Atom]gen.Atom
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
	return a.max_message_size
}

func (a *acceptor) SetMaxMessageSize(size int) {
	if size < 0 {
		size = 0
	}
	a.max_message_size = size

}

func (a *acceptor) Info() gen.AcceptorInfo {
	info := gen.AcceptorInfo{
		Interface:        a.l.Addr().String(),
		MaxMessageSize:   a.max_message_size,
		Flags:            a.flags,
		TLS:              a.cert_manager != nil,
		CustomRegistrar:  a.registrar_custom,
		HandshakeVersion: a.handshake.Version(),
		ProtoVersion:     a.proto.Version(),
	}
	regInfo := a.registrar_info()
	if regInfo.EmbeddedServer {
		info.RegistrarServer = "(embedded) " + regInfo.Server
	} else {
		info.RegistrarServer = regInfo.Server
	}
	info.RegistrarVersion = regInfo.Version
	return info
}
