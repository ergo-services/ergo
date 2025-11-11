---
description: Managing outgoing connections
---

# Static Routes

When creating an outgoing connection to a remote node, the node first checks the internal _Static Routing Table_ for an existing route. If no _static route_ is found for the specified remote node's name, the node uses the _Registrar_ and sends a resolve request to obtain the necessary connection parameters (see the [Service Discovery](service-discovering.md) section for more information).

To define a _static route_ for a specific node or group of nodes, use the `AddRoute` method of the `gen.Network` interface. This method allows you to manually configure the connection parameters for outgoing connections, ensuring more control over how the node establishes connections with the specified nodes, bypassing the need to query the _Registrar_.

```go
AddRoute(match string, route gen.NetworkRoute, weight int) error
```

* `match`: defines the name of the remote node or a pattern to match multiple nodes. The node uses the `MatchString`method from Go's standard library `regexp` package to match the node names based on this pattern.
* `route`: specifies the connection parameters to be used when establishing a connection to the matched node(s). This includes details such as the network protocol, port, TLS settings, and other connection options.
* `weight`: defines the weight of the specified route. If multiple routes match the same node name in the routing table, the node will return a list of routes sorted by descending weight. The node will then use the route with the highest weight to establish the connection.

To check for the existence of a _static route_ for a specific remote node name, use the `Route` method of the `gen.Network`interface. To remove a _static route_, use the `RemoveRoute` method of the `gen.Network` interface, specifying the `match` value that was used when adding the route.

#### gen.NetworkRoute

The `route` argument in the `AddRoute` method allows you to specify detailed connection parameters for establishing outgoing connections to remote nodes. This enables fine-grained control over how connections are created:

* `Resolver`: specifies a particular _Registrar_ to be used by the node for resolving the connection. This interface is obtained via the `Resolver` method of the `gen.Registrar` interface. This functionality is useful when a node is working with multiple network stacks. For example, see the [Erlang](../extra-library/network-protocols/erlang.md) section for managing clusters with different network stacks simultaneously.
* `Route`: allows you to explicitly define the host name, port number, TLS mode, handshake version, and protocol version. If the `Resolver` parameter is explicitly set, the route parameters provided will override any route information returned by the _Registrar_.
* `Cookie`: overrides the default `Cookie` value specified in `gen.NetworkOptions` for the given route, providing custom authentication or security settings specific to the connection.
* `Cert`: specifies a `CertManager` to establish a TLS connection, ensuring that the appropriate certificates are used during the connection.
* `Flags`: you can override specific flags for the given route. For example, you can explicitly disable certain mechanisms, such as preventing the remote node from spawning processes over the established connection.
* `AtomMapping`: enables automatic substitution of `gen.Atom` values transmitted or received during the connection.
* `LogLevel`: defines the logging level for the network stack within the established connection. This provides granular control over the verbosity of log messages for troubleshooting or monitoring network activity on a per-connection basis
