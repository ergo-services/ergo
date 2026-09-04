package registrar

import (
	"encoding/binary"
	"net"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

// fakeRegistrar answers every registration with the given bytes, whatever they mean.
func fakeRegistrar(t *testing.T, reply []byte) uint16 {
	t.Helper()
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %s", err)
	}
	t.Cleanup(func() { l.Close() })

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			conn.Read(make([]byte, 4096))
			conn.Write(reply)
		}
	}()

	return uint16(l.Addr().(*net.TCPAddr).Port)
}

func TestRegisterRefusesAMalformedReply(t *testing.T) {
	good := frame(t, protoVersion, protoRegisterReply, MessageRegisterReply{})
	tooLong := append([]byte{}, good...)
	binary.BigEndian.PutUint16(tooLong[2:4], uint16(len(tooLong)))

	for _, tc := range []struct {
		name  string
		reply []byte
	}{
		{"short reply", []byte{protoVersion, protoRegisterReply, 0}},
		{"proto version mismatch", frame(t, protoVersion+1, protoRegisterReply, MessageRegisterReply{})},
		{"unexpected reply type", frame(t, protoVersion, protoResolveReply, MessageRegisterReply{})},
		{"length past the reply", tooLong},
		{"unexpected message", frame(t, protoVersion, protoRegisterReply, MessageNodes{})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			port := fakeRegistrar(t, tc.reply)
			c := Create(Options{Port: port, DisableServer: true}).(*client)
			node := &testNode{name: "node2@localhost", log: mock.NewLog()}

			_, err := c.Register(node, gen.RegisterRoutes{Routes: []gen.Route{{Host: "localhost", Port: 7002}}})
			check.ErrorIs(t, err, gen.ErrMalformed)
		})
	}
}

func TestRegisterRefusesAnUndecodableReply(t *testing.T) {
	port := fakeRegistrar(t, []byte{protoVersion, protoRegisterReply, 0, 3, 0xff, 0xfe, 0xfd})
	c := Create(Options{Port: port, DisableServer: true}).(*client)
	node := &testNode{name: "node2@localhost", log: mock.NewLog()}

	_, err := c.Register(node, gen.RegisterRoutes{Routes: []gen.Route{{Host: "localhost", Port: 7002}}})
	if err == nil {
		t.Fatal("a reply that does not decode was accepted")
	}
}

func TestRegisterReportsTheServersRejection(t *testing.T) {
	reply := frame(t, protoVersion, protoRegisterReply, MessageRegisterReply{Error: gen.ErrTaken})
	port := fakeRegistrar(t, reply)
	c := Create(Options{Port: port, DisableServer: true}).(*client)
	node := &testNode{name: "node2@localhost", log: mock.NewLog()}

	_, err := c.Register(node, gen.RegisterRoutes{Routes: []gen.Route{{Host: "localhost", Port: 7002}}})
	check.ErrorContains(t, err, gen.ErrTaken.Error())
}

func TestReadPushKeepsTheLinkOnWhatItCannotUse(t *testing.T) {
	event := frame(t, protoVersion, protoNodeEvent, MessageNodeEvent{Node: "node3@localhost", Joined: true})

	for _, tc := range []struct {
		name      string
		push      []byte
		err       error
		published int
	}{
		{"empty body", []byte{protoVersion, protoNodeEvent, 0, 0}, nil, 0},
		{"unknown push type", frame(t, protoVersion, 99, MessageNodes{}), nil, 0},
		{"undecodable body", []byte{protoVersion, protoNodeEvent, 0, 3, 0xff, 0xfe, 0xfd}, nil, 0},
		{"unexpected message", frame(t, protoVersion, protoNodeEvent, MessageNodes{}), nil, 0},
		{"proto version mismatch", []byte{protoVersion + 1, protoNodeEvent, 0, 0}, gen.ErrMalformed, 0},
		{"node event", event, nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := &testNode{name: "node2@localhost", log: mock.NewLog()}
			c := Create(Options{}).(*client)
			c.node = node
			c.event = "test_event"

			here, there := net.Pipe()
			defer here.Close()
			defer there.Close()
			go there.Write(tc.push)

			err := c.readPush(here)
			if tc.err == nil {
				check.NoError(t, err)
			} else {
				check.ErrorIs(t, err, tc.err)
			}
			check.Equal(t, tc.published, len(node.sent()))
		})
	}
}

func TestReadPushFailsOnAClosedLink(t *testing.T) {
	c := Create(Options{}).(*client)
	c.node = &testNode{name: "node2@localhost", log: mock.NewLog()}

	here, there := net.Pipe()
	there.Close()

	if err := c.readPush(here); err == nil {
		t.Fatal("reading a closed link answered without an error")
	}
}
