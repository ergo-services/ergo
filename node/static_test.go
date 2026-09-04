package node

import (
	"crypto/tls"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/mock"
)

func TestStaticRoutesAddRemove(t *testing.T) {
	var srs staticRoutes

	if err := srs.add("node1@.*", gen.NetworkRoute{}, 10); err != nil {
		t.Fatalf("add: %v", err)
	}
	// the same match twice is rejected
	if err := srs.add("node1@.*", gen.NetworkRoute{}, 5); err != gen.ErrTaken {
		t.Fatalf("duplicate add = %v, want ErrTaken", err)
	}
	// an invalid regexp surfaces the compile error (not ErrTaken)
	if err := srs.add("[unbalanced", gen.NetworkRoute{}, 1); err == nil || err == gen.ErrTaken {
		t.Fatalf("invalid regexp add = %v, want a compile error", err)
	}

	if err := srs.remove("node1@.*"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := srs.remove("node1@.*"); err != gen.ErrUnknown {
		t.Fatalf("remove missing = %v, want ErrUnknown", err)
	}
}

func TestStaticRoutesLookup(t *testing.T) {
	var srs staticRoutes

	// nothing registered: no match
	if routes, found := srs.lookup("anything"); found || routes != nil {
		t.Fatalf("lookup on empty = (%v, %v), want (nil, false)", routes, found)
	}

	// two routes match "abc", one does not; higher weight comes first
	srs.add("a.*", gen.NetworkRoute{Route: gen.Route{Host: "low"}}, 5)
	srs.add("ab.*", gen.NetworkRoute{Route: gen.Route{Host: "high"}}, 10)
	srs.add("z.*", gen.NetworkRoute{Route: gen.Route{Host: "nope"}}, 99)

	routes, found := srs.lookup("abc")
	if found == false {
		t.Fatal("lookup abc: not found")
	}
	if len(routes) != 2 {
		t.Fatalf("lookup abc returned %d routes, want 2", len(routes))
	}
	if routes[0].Route.Host != "high" || routes[1].Route.Host != "low" {
		t.Fatalf("lookup order = [%s %s], want [high low]", routes[0].Route.Host, routes[1].Route.Host)
	}

	// a name matching no pattern
	if _, found := srs.lookup("qqq"); found {
		t.Fatal("lookup qqq: should not match")
	}
}

func TestStaticRoutesInfo(t *testing.T) {
	var srs staticRoutes

	flags := gen.NetworkFlags{Enable: true, EnableRemoteSpawn: true}
	srs.add("rich@.*", gen.NetworkRoute{
		Resolver: mock.NewResolver(),
		Cookie:   "secret",
		Cert:     gen.CreateCertManager(tls.Certificate{}),
		Flags:    flags,
		Route: gen.Route{
			Host:             "example.com",
			Port:             4370,
			HandshakeVersion: gen.Version{Name: "hs"},
			ProtoVersion:     gen.Version{Name: "edf"},
		},
	}, 7)
	srs.add("bare@.*", gen.NetworkRoute{}, 0)

	byMatch := map[string]gen.RouteInfo{}
	for _, ri := range srs.info() {
		byMatch[ri.Match] = ri
	}
	if len(byMatch) != 2 {
		t.Fatalf("info returned %d entries, want 2", len(byMatch))
	}

	rich := byMatch["rich@.*"]
	if rich.Weight != 7 || rich.Host != "example.com" || rich.Port != 4370 {
		t.Fatalf("rich basics = %+v", rich)
	}
	if rich.UseResolver == false || rich.UseCustomCookie == false || rich.UseCustomCert == false {
		t.Fatalf("rich capability flags = %+v", rich)
	}
	if rich.Flags != flags {
		t.Fatalf("rich flags = %+v, want %+v", rich.Flags, flags)
	}
	if rich.HandshakeVersion.Name != "hs" || rich.ProtoVersion.Name != "edf" {
		t.Fatalf("rich versions = %+v / %+v", rich.HandshakeVersion, rich.ProtoVersion)
	}

	bare := byMatch["bare@.*"]
	if bare.UseResolver || bare.UseCustomCookie || bare.UseCustomCert {
		t.Fatalf("bare capability flags = %+v, want all false", bare)
	}
}

func TestStaticProxiesAddRemove(t *testing.T) {
	var sps staticProxies

	if err := sps.add("node1@.*", gen.NetworkProxyRoute{}, 10); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := sps.add("node1@.*", gen.NetworkProxyRoute{}, 5); err != gen.ErrTaken {
		t.Fatalf("duplicate add = %v, want ErrTaken", err)
	}
	if err := sps.add("[unbalanced", gen.NetworkProxyRoute{}, 1); err == nil || err == gen.ErrTaken {
		t.Fatalf("invalid regexp add = %v, want a compile error", err)
	}

	if err := sps.remove("node1@.*"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := sps.remove("node1@.*"); err != gen.ErrUnknown {
		t.Fatalf("remove missing = %v, want ErrUnknown", err)
	}
}

func TestStaticProxiesLookup(t *testing.T) {
	var sps staticProxies

	if routes, found := sps.lookup("anything"); found || routes != nil {
		t.Fatalf("lookup on empty = (%v, %v), want (nil, false)", routes, found)
	}

	sps.add("a.*", gen.NetworkProxyRoute{Route: gen.ProxyRoute{Proxy: "low"}}, 5)
	sps.add("ab.*", gen.NetworkProxyRoute{Route: gen.ProxyRoute{Proxy: "high"}}, 10)
	sps.add("z.*", gen.NetworkProxyRoute{Route: gen.ProxyRoute{Proxy: "nope"}}, 99)

	routes, found := sps.lookup("abc")
	if found == false {
		t.Fatal("lookup abc: not found")
	}
	if len(routes) != 2 {
		t.Fatalf("lookup abc returned %d routes, want 2", len(routes))
	}
	if routes[0].Route.Proxy != "high" || routes[1].Route.Proxy != "low" {
		t.Fatalf("lookup order = [%s %s], want [high low]", routes[0].Route.Proxy, routes[1].Route.Proxy)
	}

	if _, found := sps.lookup("qqq"); found {
		t.Fatal("lookup qqq: should not match")
	}
}

func TestStaticProxiesInfo(t *testing.T) {
	var sps staticProxies

	flags := gen.NetworkProxyFlags{Enable: true, EnableEncryption: true}
	sps.add("rich@.*", gen.NetworkProxyRoute{
		Resolver: mock.NewResolver(),
		Cookie:   "secret",
		Flags:    flags,
		MaxHop:   4,
		Route:    gen.ProxyRoute{Proxy: "proxy@host"},
	}, 7)
	sps.add("bare@.*", gen.NetworkProxyRoute{}, 0)

	byMatch := map[string]gen.ProxyRouteInfo{}
	for _, rpi := range sps.info() {
		byMatch[rpi.Match] = rpi
	}
	if len(byMatch) != 2 {
		t.Fatalf("info returned %d entries, want 2", len(byMatch))
	}

	rich := byMatch["rich@.*"]
	if rich.Weight != 7 || rich.MaxHop != 4 || rich.Proxy != "proxy@host" {
		t.Fatalf("rich basics = %+v", rich)
	}
	if rich.UseResolver == false || rich.UseCustomCookie == false {
		t.Fatalf("rich capability flags = %+v", rich)
	}
	if rich.Flags != flags {
		t.Fatalf("rich flags = %+v, want %+v", rich.Flags, flags)
	}

	bare := byMatch["bare@.*"]
	if bare.UseResolver || bare.UseCustomCookie {
		t.Fatalf("bare capability flags = %+v, want all false", bare)
	}
}
