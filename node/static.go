package node

import (
	"regexp"
	"sort"
	"sync"

	"ergo.services/ergo/gen"
)

type staticRoutes struct {
	routes sync.Map // match => staticRoute
}

type staticRoute struct {
	re     *regexp.Regexp
	route  gen.NetworkRoute
	weight int
}

func (srs *staticRoutes) add(match string, route gen.NetworkRoute, weight int) error {
	re, err := regexp.Compile(match)
	if err != nil {
		return err
	}

	sr := staticRoute{
		re:     re,
		route:  route,
		weight: weight,
	}
	_, exist := srs.routes.LoadOrStore(match, sr)
	if exist {
		return gen.ErrTaken
	}
	return nil
}

func (srs *staticRoutes) remove(match string) error {
	_, found := srs.routes.LoadAndDelete(match)
	if found == false {
		return gen.ErrUnknown
	}
	return nil
}

func (srs *staticRoutes) lookup(name string) ([]gen.NetworkRoute, bool) {
	var sroutes []staticRoute
	var routes []gen.NetworkRoute

	srs.routes.Range(func(_, v any) bool {
		sr := v.(staticRoute)
		if match := sr.re.MatchString(name); match {
			sroutes = append(sroutes, sr)
		}
		return true
	})

	if len(sroutes) == 0 {
		return nil, false
	}

	sort.Slice(sroutes, func(i, j int) bool {
		return sroutes[i].weight > sroutes[j].weight
	})

	for _, sr := range sroutes {
		routes = append(routes, sr.route)
	}

	return routes, true
}

func (srs *staticRoutes) info() []gen.RouteInfo {
	var info []gen.RouteInfo
	srs.routes.Range(func(k, v any) bool {
		sr := v.(staticRoute)
		ri := gen.RouteInfo{
			Match:            k.(string),
			Weight:           sr.weight,
			UseResolver:      sr.route.Resolver != nil,
			UseCustomCookie:  sr.route.Cookie != "",
			UseCustomCert:    sr.route.Cert != nil,
			Flags:            sr.route.Flags,
			HandshakeVersion: sr.route.Route.HandshakeVersion,
			ProtoVersion:     sr.route.Route.ProtoVersion,
			Host:             sr.route.Route.Host,
			Port:             sr.route.Route.Port,
		}
		info = append(info, ri)
		return true
	})
	return info
}

type staticProxies struct {
	routes sync.Map // match => staticProxy
}

type staticProxy struct {
	re     *regexp.Regexp
	route  gen.NetworkProxyRoute
	weight int
}

func (sps *staticProxies) add(match string, route gen.NetworkProxyRoute, weight int) error {
	re, err := regexp.Compile(match)
	if err != nil {
		return err
	}

	sp := staticProxy{
		re:     re,
		route:  route,
		weight: weight,
	}
	_, exist := sps.routes.LoadOrStore(match, sp)
	if exist {
		return gen.ErrTaken
	}
	return nil
}

func (sps *staticProxies) remove(match string) error {
	_, found := sps.routes.LoadAndDelete(match)
	if found == false {
		return gen.ErrUnknown
	}
	return nil
}

func (sps *staticProxies) lookup(name string) ([]gen.NetworkProxyRoute, bool) {
	var sroutes []staticProxy
	var routes []gen.NetworkProxyRoute

	sps.routes.Range(func(_, v any) bool {
		sp := v.(staticProxy)
		if match := sp.re.MatchString(name); match {
			sroutes = append(sroutes, sp)
		}
		return true
	})

	if len(sroutes) == 0 {
		return nil, false
	}

	sort.Slice(sroutes, func(i, j int) bool {
		return sroutes[i].weight > sroutes[j].weight
	})

	for _, sr := range sroutes {
		routes = append(routes, sr.route)
	}

	return routes, true
}

func (sps *staticProxies) info() []gen.ProxyRouteInfo {
	var info []gen.ProxyRouteInfo
	sps.routes.Range(func(k, v any) bool {
		sp := v.(staticProxy)
		rpi := gen.ProxyRouteInfo{
			Match:           k.(string),
			Weight:          sp.weight,
			UseResolver:     sp.route.Resolver != nil,
			UseCustomCookie: sp.route.Cookie != "",
			Flags:           sp.route.Flags,
			MaxHop:          sp.route.MaxHop,
			Proxy:           sp.route.Route.Proxy,
		}
		info = append(info, rpi)
		return true
	})
	return info
}
