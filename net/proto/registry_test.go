package proto

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/net/edf"
	"ergo.services/ergo/net/handshake"
	"ergo.services/ergo/testing/mock"
)

var testRegistry = func() *edf.Registry {
	r := edf.NewRegistry()
	if err := r.RegisterTypes(protoTypes); err != nil {
		panic(err)
	}
	return r
}()

func TestNewConnectionNeedsRegistry(t *testing.T) {
	result := gen.HandshakeResult{
		ConnectionID: "conn-1",
		Peer:         "peer@localhost",
		PeerCreation: 1,
		Custom:       handshake.ConnectionOptions{PoolSize: 1},
	}
	if _, err := (&enp{}).NewConnection(mock.NewCore(), result, mock.NewLog()); err == nil {
		t.Fatal("a connection without a registry must be refused")
	}
}

func TestNewConnectionUsesRegistry(t *testing.T) {
	result := gen.HandshakeResult{
		ConnectionID: "conn-1",
		Peer:         "peer@localhost",
		PeerCreation: 1,
		Custom:       handshake.ConnectionOptions{PoolSize: 1, Registry: testRegistry},
	}
	conn, err := (&enp{}).NewConnection(mock.NewCore(), result, mock.NewLog())
	if err != nil {
		t.Fatal(err)
	}
	c := conn.(*connection)
	if c.encodeOptions.Registry != testRegistry || c.decodeOptions.Registry != testRegistry {
		t.Fatal("both directions must use the registry of the handshake")
	}
}
