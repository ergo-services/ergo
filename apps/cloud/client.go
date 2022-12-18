package cloud

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
)

type CloudNode struct {
	Node       string
	Port       uint16
	SkipVerify bool
}

type cloudClient struct {
	gen.Server
}

type cloudClientState struct {
	options   node.Cloud
	handshake node.HandshakeInterface
	monitor   etf.Ref
	node      string
}

type messageCloudClientConnect struct{}

func (cc *cloudClient) Init(process *gen.ServerProcess, args ...etf.Term) error {
	lib.Log("[%s] CLOUD_CLIENT: Init: %#v", process.NodeName(), args)
	if len(args) == 0 {
		return fmt.Errorf("no args to start cloud client")
	}

	cloudOptions, ok := args[0].(node.Cloud)
	if ok == false {
		return fmt.Errorf("wrong args for the cloud client")
	}

	handshake, err := createHandshake(cloudOptions)
	if err != nil {
		return fmt.Errorf("can not create HandshakeInterface for the cloud client: %s", err)
	}

	process.State = &cloudClientState{
		options:   cloudOptions,
		handshake: handshake,
	}

	if err := process.RegisterEvent(EventCloud, MessageEventCloud{}); err != nil {
		lib.Warning("can't register event %q: %s", EventCloud, err)
	}

	process.Cast(process.Self(), messageCloudClientConnect{})

	return nil
}

func (cc *cloudClient) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	lib.Log("[%s] CLOUD_CLIENT: HandleCast: %#v", process.NodeName(), message)
	switch message.(type) {
	case messageCloudClientConnect:
		state := process.State.(*cloudClientState)

		// initiate connection with the cloud
		cloudNodes, err := getCloudNodes()
		if err != nil {
			lib.Warning("can't resolve cloud nodes: %s", err)
		}

		// add static route with custom handshake
		thisNode := process.Env(node.EnvKeyNode).(node.Node)

		for _, cloud := range cloudNodes {
			routeOptions := node.RouteOptions{
				Cookie:    state.options.Cookie,
				IsErgo:    true,
				Handshake: state.handshake,
			}
			routeOptions.TLS = &tls.Config{
				InsecureSkipVerify: cloud.SkipVerify,
			}
			if err := thisNode.AddStaticRoutePort(cloud.Node, cloud.Port, routeOptions); err != nil {
				if err != lib.ErrTaken {
					continue
				}
			}

			lib.Log("[%s] CLOUD_CLIENT: trying to connect with: %s", process.NodeName(), cloud.Node)
			if err := thisNode.Connect(cloud.Node); err != nil {
				lib.Log("[%s] CLOUD_CLIENT: failed with reason: ", err)
				continue
			}

			// add proxy domain route
			proxyRoute := node.ProxyRoute{
				Name:   "@" + state.options.Cluster,
				Proxy:  cloud.Node,
				Cookie: state.options.Cookie,
			}
			thisNode.AddProxyRoute(proxyRoute)

			state.monitor = process.MonitorNode(cloud.Node)
			state.node = cloud.Node
			event := MessageEventCloud{
				Cluster: proxyRoute.Name,
				Online:  true,
				Proxy:   cloud.Node,
			}
			if err := process.SendEventMessage(EventCloud, event); err != nil {
				lib.Log("[%s] CLOUD_CLIENT: failed to send event (%s) %#v: %s",
					process.NodeName(), EventCloud, event, err)
			}
			return gen.ServerStatusOK
		}

		// cloud nodes aren't available. make another attempt in 3 seconds
		process.CastAfter(process.Self(), messageCloudClientConnect{}, 5*time.Second)
	}
	return gen.ServerStatusOK
}

func (cc *cloudClient) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	lib.Log("[%s] CLOUD_CLIENT: HandleInfo: %#v", process.NodeName(), message)
	state := process.State.(*cloudClientState)

	switch m := message.(type) {
	case gen.MessageNodeDown:
		if m.Ref != state.monitor {
			return gen.ServerStatusOK
		}
		thisNode := process.Env(node.EnvKeyNode).(node.Node)
		state.cleanup(thisNode)

		event := MessageEventCloud{
			Online: false,
		}
		process.SendEventMessage(EventCloud, event)
		// lost connection with the cloud node. try to connect again
		process.Cast(process.Self(), messageCloudClientConnect{})
	}
	return gen.ServerStatusOK
}

func (cc *cloudClient) Terminate(process *gen.ServerProcess, reason string) {
	state := process.State.(*cloudClientState)
	thisNode := process.Env(node.EnvKeyNode).(node.Node)
	thisNode.RemoveProxyRoute("@" + state.options.Cluster)
	thisNode.Disconnect(state.node)
	state.cleanup(thisNode)
}

func (ccs *cloudClientState) cleanup(node node.Node) {
	node.RemoveStaticRoute(ccs.node)
	node.RemoveProxyRoute("@" + ccs.options.Cluster)
	ccs.node = ""
}

func getCloudNodes() ([]CloudNode, error) {
	// check if custom cloud entries have been defined via env
	if entries := strings.Fields(os.Getenv("ERGO_SERVICES_CLOUD")); len(entries) > 0 {
		nodes := []CloudNode{}
		for _, entry := range entries {
			re := regexp.MustCompile("[@:]+")
			nameHostPort := re.Split(entry, -1)
			name := "dist"
			host := "localhost"
			port := 4411
			switch len(nameHostPort) {
			case 2:
				// either abc@def or abc:def
				if p, err := strconv.Atoi(nameHostPort[1]); err == nil {
					port = p
				} else {
					name = nameHostPort[0]
					host = nameHostPort[1]
				}
			case 3:
				if p, err := strconv.Atoi(nameHostPort[2]); err == nil {
					port = p
				} else {
					continue
				}
				name = nameHostPort[0]
				host = nameHostPort[1]

			default:
				continue
			}

			node := CloudNode{
				Node:       name + "@" + host,
				Port:       uint16(port),
				SkipVerify: true,
			}
			nodes = append(nodes, node)

		}

		if len(nodes) > 0 {
			return nodes, nil
		}
	}
	_, srv, err := net.LookupSRV("cloud", "dist", "ergo.services")
	if err != nil {
		return nil, err
	}
	nodes := make([]CloudNode, len(srv))
	for i := range srv {
		nodes[i].Node = "dist@" + strings.TrimSuffix(srv[i].Target, ".")
		nodes[i].Port = srv[i].Port
	}
	return nodes, nil
}
