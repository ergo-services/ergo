package registrar

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/edf"
)

type server struct {
	sync.RWMutex

	lReg net.Listener
	lRes net.PacketConn

	log gen.Log

	routes     map[gen.Atom][]gen.Route
	registered map[net.Conn]gen.Atom
	terminated bool
}

func tryStartServer(port uint16, log gen.Log) *server {
	addressReg := fmt.Sprintf("localhost:%d", port)
	lReg, err := net.Listen("tcp", addressReg)
	if err != nil {
		// might be already taken. dont care
		return nil
	}
	addressRes := fmt.Sprintf(":%d", port)
	lRes, err := net.ListenPacket("udp", addressRes)
	if err != nil {
		// might be already taken. dont care
		lReg.Close()
		return nil
	}

	srv := &server{
		lReg:       lReg,
		lRes:       lRes,
		log:        log,
		routes:     make(map[gen.Atom][]gen.Route),
		registered: make(map[net.Conn]gen.Atom),
	}

	go srv.serveRegister()
	go srv.serveResolve()

	srv.log.Trace("(registrar) server started on tcp://%s and resolver on udp://%s",
		lReg.Addr(), lRes.LocalAddr())
	return srv
}

func (s *server) serveRegister() {
	for {
		if s.terminated {
			return
		}
		conn, err := s.lReg.Accept()
		if err != nil {
			return
		}

		go s.serveConn(conn)
	}

}

func (s *server) serveResolve() {
	buf := make([]byte, 1024)
	rbuf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(rbuf)

	for {
		if s.terminated {
			return
		}

		n, addr, err := s.lRes.ReadFrom(buf)
		if err != nil {
			s.log.Trace("(registrar) unable to read from UDP socket: %s", err)
			continue
		}
		// decode request
		s.log.Trace("(registrar) got resolve request from %s", addr)
		if n < 4 {
			s.log.Error("(registrar) malformed data from %s", addr)
			continue
		}

		if buf[0] != protoVersion {
			s.log.Error("(registrar) proto version mismatch from %s: %d", addr, buf[0])
			continue
		}

		if buf[1] != protoResolve {
			s.log.Error("(registrar) unknown UDP packet type from %s: %d", addr, buf[1])
			continue
		}

		l := binary.BigEndian.Uint16(buf[2:4])
		if int(l) > n-4 {
			s.log.Error("(registrar) malformed data from %s", addr)
			continue
		}

		v, _, err := edf.Decode(buf[4:], edf.Options{})
		if err != nil {
			s.log.Error("(registrar) unable to decode message from %s: %s", addr, err)
			continue
		}

		resolve, ok := v.(MessageResolveRoutes)
		if ok == false {
			s.log.Error("(registrar) incorrect <register route> message from %s: %#v", addr, v)
			continue
		}

		// resolve and send reply
		routes, err := s.resolve(resolve.Node, false)
		reply := MessageResolveReply{
			Routes: routes,
			Error:  err,
		}

		rbuf.Reset()
		rbuf.Allocate(4)
		rbuf.B[0] = protoVersion
		rbuf.B[1] = protoResolveReply
		if err := edf.Encode(reply, rbuf, edf.Options{}); err != nil {
			s.log.Error("(registrar) unable to encode resolve reply message: %s", err)
			continue
		}
		binary.BigEndian.PutUint16(rbuf.B[2:4], uint16(rbuf.Len()-4))
		if _, err := s.lRes.WriteTo(rbuf.B, addr); err != nil {
			s.log.Error("(registrar) unable to send resolve reply message: %s", err)
		}
	}
}

func (s *server) terminate() {
	s.terminated = true

	s.lReg.Close()
	s.lRes.Close()

	s.RLock()
	defer s.RUnlock()

	for k := range s.registered {
		k.(net.Conn).Close()
	}

	s.log.Trace("(registrar) server terminated")
}

func (s *server) serveConn(conn net.Conn) {
	s.log.Trace("(registrar) server got connection from %s", conn.RemoteAddr())

	conn.SetReadDeadline(time.Now().Add(time.Second))

	buf := make([]byte, 4096) // must be enough for the register packet
	n, err := conn.Read(buf)
	if err != nil {
		s.log.Error("(registrar) unable to read from socket with %s: %s", conn.RemoteAddr(), err)
		conn.Close()
		return
	}
	if n < 4 {
		s.log.Error("(registrar) malformed data from %s", conn.RemoteAddr())
		conn.Close()
		return
	}

	if buf[0] != protoVersion {
		s.log.Error("(registrar) proto version mismatch from %s: %d", conn.RemoteAddr(), buf[0])
		conn.Close()
		return
	}

	if buf[1] != protoRegister {
		s.log.Error("(registrar) unknown packet type from %s: %d", conn.RemoteAddr(), buf[1])
		conn.Close()
		return
	}

	l := binary.BigEndian.Uint16(buf[2:4])
	if int(l) > n-4 {
		s.log.Error("(registrar) malformed data from %s", conn.RemoteAddr())
		conn.Close()
		return
	}

	v, _, err := edf.Decode(buf[4:], edf.Options{})
	if err != nil {
		s.log.Error("(registrar) unable to decode message from %s: %s", conn.RemoteAddr(), err)
		conn.Close()
		return
	}
	routes, ok := v.(MessageRegisterRoutes)
	if ok == false {
		s.log.Error("(registrar) incorrect <register route> message from %s: %#v", conn.RemoteAddr(), v)
		conn.Close()
		return
	}

	reply := MessageRegisterReply{
		Error: s.registerNode(routes.Node, routes.Routes, conn),
	}

	rbuf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(rbuf)

	rbuf.Allocate(4)
	rbuf.B[0] = protoVersion
	rbuf.B[1] = protoRegisterReply
	if err := edf.Encode(reply, rbuf, edf.Options{}); err != nil {
		conn.Close()
		return
	}
	binary.BigEndian.PutUint16(rbuf.B[2:4], uint16(rbuf.Len()-4))
	if _, err := conn.Write(rbuf.B); err != nil {
		return
	}

	if reply.Error != nil {
		conn.Close()
		return
	}

	conn.SetReadDeadline(time.Time{})
	defer s.unregisterNode(routes.Node, conn)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}

		if n > 0 {
			s.log.Warning("(registrar) misbehavior in reg link with %s, received %d bytes. ignored", routes.Node, n)
		}
	}
}

func (s *server) registerNode(name gen.Atom, routes []gen.Route, conn net.Conn) error {
	s.Lock()
	defer s.Unlock()
	if _, found := s.routes[name]; found {
		s.log.Trace("(registrar) unable to register %s: %s", name, gen.ErrTaken)
		return gen.ErrTaken
	}
	if len(routes) == 0 {
		return gen.ErrIncorrect
	}

	s.routes[name] = routes
	if conn != nil {
		s.registered[conn] = name
	}
	s.log.Trace("(registrar) registered node %s with %d route(s)", name, len(routes))
	return nil
}

func (s *server) unregisterNode(name gen.Atom, conn net.Conn) {
	s.Lock()
	defer s.Unlock()
	if _, found := s.routes[name]; found == false {
		return
	}
	delete(s.routes, name)
	delete(s.registered, conn)

	s.log.Trace("(registrar) unregistered node %s", name)
}

func (s *server) resolve(name gen.Atom, docopy bool) ([]gen.Route, error) {
	var routes []gen.Route

	s.RLock()
	defer s.RUnlock()
	if regs, found := s.routes[name]; found {
		if docopy == false {
			return regs, nil
		}
		for _, r := range regs {
			routes = append(routes, r)
		}
		return routes, nil
	}
	return nil, gen.ErrUnknown
}
