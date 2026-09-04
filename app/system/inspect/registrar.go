package inspect

import "ergo.services/ergo/gen"

// The registrar view of this node. A caller asking a remote node gets that node's
// picture of the cluster, which is what makes a map of a cluster the caller is not
// a member of possible. Every answer carries the error rather than an empty result:
// "no registrar", "feature unsupported" and "nothing found" must stay distinct.

func (i *inspect) responseRegistrarNodes() ResponseGetRegistrarNodes {
	registrar, err := i.Node().Network().Registrar()
	if err != nil {
		return ResponseGetRegistrarNodes{Error: err}
	}
	nodes, err := registrar.Nodes()
	return ResponseGetRegistrarNodes{Nodes: nodes, Error: err}
}

func (i *inspect) responseRegistrarRoutes(request RequestGetRegistrarRoutes) ResponseGetRegistrarRoutes {
	if request.Node == "" {
		return ResponseGetRegistrarRoutes{Error: gen.ErrIncorrect}
	}
	registrar, err := i.Node().Network().Registrar()
	if err != nil {
		return ResponseGetRegistrarRoutes{Error: err}
	}
	routes, err := registrar.Resolver().Resolve(request.Node)
	return ResponseGetRegistrarRoutes{Routes: routes, Error: err}
}

func (i *inspect) responseRegistrarProxyRoutes(request RequestGetRegistrarProxyRoutes) ResponseGetRegistrarProxyRoutes {
	if request.Node == "" {
		return ResponseGetRegistrarProxyRoutes{Error: gen.ErrIncorrect}
	}
	registrar, err := i.Node().Network().Registrar()
	if err != nil {
		return ResponseGetRegistrarProxyRoutes{Error: err}
	}
	routes, err := registrar.Resolver().ResolveProxy(request.Node)
	return ResponseGetRegistrarProxyRoutes{Routes: routes, Error: err}
}

func (i *inspect) responseRegistrarApplicationRoutes(request RequestGetRegistrarApplicationRoutes) ResponseGetRegistrarApplicationRoutes {
	if request.Name == "" {
		return ResponseGetRegistrarApplicationRoutes{Error: gen.ErrIncorrect}
	}
	registrar, err := i.Node().Network().Registrar()
	if err != nil {
		return ResponseGetRegistrarApplicationRoutes{Error: err}
	}
	routes, err := registrar.Resolver().ResolveApplication(request.Name)
	// filtering by tags and state stays with the caller, so the whole set travels
	return ResponseGetRegistrarApplicationRoutes{Routes: routes, Error: err}
}
