package node

import (
	"sync"
	"testing"

	"ergo.services/ergo/gen"
)

func TestNetworkStateSettersRace(t *testing.T) {
	n := &network{}
	n.cookie.Store(new(string))
	n.flags.Store(&gen.NetworkFlags{})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			n.SetCookie("cookie")
			n.SetMaxMessageSize(i)
			n.SetNetworkFlags(gen.NetworkFlags{Enable: true})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			_ = n.Cookie()
			_ = n.MaxMessageSize()
			_ = n.NetworkFlags()
		}
	}()
	wg.Wait()
}

func TestNetworkReadersGuardOnReady(t *testing.T) {
	n := &network{}
	n.running.Store(true) // claimed, but start() has not published the state yet

	if _, err := n.Registrar(); err != gen.ErrNetworkStopped {
		t.Fatalf("Registrar in the start window: got %v, want ErrNetworkStopped", err)
	}
	if _, err := n.Info(); err != gen.ErrNetworkStopped {
		t.Fatalf("Info in the start window must not deref the nil registrar: got %v, want ErrNetworkStopped", err)
	}
	if _, err := n.Acceptors(); err != gen.ErrNetworkStopped {
		t.Fatalf("Acceptors in the start window: got %v, want ErrNetworkStopped", err)
	}
}
