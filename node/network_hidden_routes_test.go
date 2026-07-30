package node

import (
	"fmt"
	"strings"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/mock"
)

func TestNetworkHiddenStaticRoutesClearedOnStop(t *testing.T) {
	var logs []string
	lg := createLog(gen.LogLevelError, func(m gen.MessageLog, logger string) {
		logs = append(logs, fmt.Sprintf(m.Format, m.Args...))
	})
	nd := &node{name: "hidden@localhost", log: lg}
	n := createNetwork(nd)

	reg := mock.NewRegistrar()
	reg.OnRegister(func(gen.NodeRegistrar, gen.RegisterRoutes) (gen.StaticRoutes, error) {
		return gen.StaticRoutes{
			Routes:  map[string]gen.NetworkRoute{"worker@.+": {}},
			Proxies: map[string]gen.NetworkProxyRoute{"proxy@.+": {}},
		}, nil
	})

	opts := gen.NetworkOptions{Mode: gen.NetworkModeHidden, Cookie: "cookie", Registrar: reg}

	// a user-added route that must survive a stop/start cycle
	if err := n.AddRoute("manual@.+", gen.NetworkRoute{}, 0); err != nil {
		t.Fatal(err)
	}

	if err := n.start(opts); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if _, ok := n.staticRoutes.lookup("worker@localhost"); ok == false {
		t.Fatal("registrar route not added on first start")
	}
	if _, ok := n.staticProxies.lookup("proxy@localhost"); ok == false {
		t.Fatal("registrar proxy route not added on first start")
	}
	if _, ok := n.staticRoutes.lookup("manual@localhost"); ok == false {
		t.Fatal("user route missing after first start")
	}

	if err := n.stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if _, ok := n.staticRoutes.lookup("worker@localhost"); ok {
		t.Fatal("registrar route not cleared on stop")
	}
	if _, ok := n.staticProxies.lookup("proxy@localhost"); ok {
		t.Fatal("registrar proxy route not cleared on stop")
	}
	if _, ok := n.staticRoutes.lookup("manual@localhost"); ok == false {
		t.Fatal("user route wrongly cleared on stop")
	}

	logs = nil
	if err := n.start(opts); err != nil {
		t.Fatalf("second start: %v", err)
	}
	if _, ok := n.staticRoutes.lookup("worker@localhost"); ok == false {
		t.Fatal("registrar route not re-added on second start")
	}
	for _, s := range logs {
		if strings.Contains(s, "unable to add") {
			t.Fatalf("ErrTaken log noise on restart: %q", s)
		}
	}
}
