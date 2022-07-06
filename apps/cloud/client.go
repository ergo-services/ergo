package cloud

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
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
	options node.Cloud
	monitor etf.Ref
	node    string
}

type messageCloudClientConnect struct{}

func (cc *cloudClient) Init(process *gen.ServerProcess, args ...etf.Term) error {
	lib.Log("CLOUD_CLIENT: Init: %#v", args)
	if len(args) == 0 {
		return fmt.Errorf("no args to start cloud client")
	}

	cloudOptions, ok := args[0].(node.Cloud)
	if ok == false {
		return fmt.Errorf("wrong args for the cloud client")
	}

	process.State = &cloudClientState{
		options: cloudOptions,
	}

	if err := process.RegisterEvent(EventCloud, []gen.EventMessage{MessageEventCloud{}}); err != nil {
		lib.Warning("can't register event %q: %s", EventCloud, err)
	}

	process.Cast(process.Self(), messageCloudClientConnect{})

	return nil
}

func (cc *cloudClient) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	lib.Log("CLOUD_CLIENT: HandleCast: %#v", message)
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
				IsErgo:    true,
				Handshake: createHandshake(state.options),
			}
			routeOptions.TLS = &tls.Config{
				InsecureSkipVerify: cloud.SkipVerify,
			}
			if err := thisNode.AddStaticRoutePort(cloud.Node, cloud.Port, routeOptions); err != nil {
				if err != lib.ErrTaken {
					continue
				}
			}

			if err := thisNode.Connect(cloud.Node); err != nil {
				continue
			}

			event := MessageEventCloud{
				Online: true,
			}
			process.SendEventMessage(EventCloud, event)
			state.monitor = process.MonitorNode(cloud.Node)
			state.node = cloud.Node
			return gen.ServerStatusOK
		}

		// cloud nodes aren't available. make another attempt in 3 seconds
		process.CastAfter(process.Self(), messageCloudClientConnect{}, 5*time.Second)
	}
	return gen.ServerStatusOK
}

func (cc *cloudClient) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	lib.Log("CLOUD_CLIENT: HandleInfo: %#v", message)
	state := process.State.(*cloudClientState)

	switch m := message.(type) {
	case gen.MessageDown:
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
	thisNode.Disconnect(state.node)
	state.cleanup(thisNode)
}

func (ccs *cloudClientState) cleanup(node node.Node) {
	node.RemoveStaticRoute(ccs.node)
	ccs.node = ""
}

func getCloudNodes() ([]CloudNode, error) {
	// check if custom cloud entries have been defined via env
	if entries := strings.Fields(os.Getenv("ERGO_SERVICES_CLOUD")); len(entries) > 0 {
		nodes := []CloudNode{}
		for _, entry := range entries {
			hostport := strings.Split(entry, ":")
			if len(hostport) != 2 {
				continue
			}

			port, err := strconv.Atoi(hostport[1])
			if err != nil {
				continue
			}

			host := "localhost"
			if hostport[0] != "" {
			}

			node := CloudNode{
				Node:       "dist@" + host,
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
