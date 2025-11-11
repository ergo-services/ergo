---
description: Registrar
---

# Service Discovering

The _Service Discovering_ mechanism allows nodes to automatically find other nodes and determine connection parameters.&#x20;

Each node has a _Registrar_ that starts during the node's initialization and operates in either server or client mode. In Ergo Framework, the default _Registrar_ implementation is available in the package `ergo.services/ergo/net/registrar`. If you are using [Saturn](../tools/saturn.md), you need to use the corresponding client available in `ergo.services/registrar/saturn`. For communication with [Erlang](../extra-library/network-protocols/erlang.md) nodes, you should use the implementation from `ergo.services/proto/erlang23/epmd`.

The default _Registrar_, when running in server mode, opens a TCP socket on `localhost:4499` to register other nodes on the host and a UDP socket on `*:4499` to handle resolve requests from other nodes, including remote ones. If the _Registrar_ fails to start in server mode, it switches to client mode and registers with the _Registrar_ on the other node at same host which is running in server-mode.

When a node starts, it establishes a TCP connection with the _Registrar_ server, which remains open until the node terminates. During registration, your node communicates its name, information about all acceptors, and their parameters (port number, Handshake/Proto version, TLS flag) to the _Registrar_. If using [Saturn](../extra-library/registrars/saturn-client.md), additional information about running applications on the node is also communicated (this functionality is not supported by the built-in _Registrar_).

When a node running the _Registrar_ in server mode terminates, other nodes on the same host automatically attempt to switch to server mode. The node that first successfully opens the TCP socket on `localhost:4499` will switch to server mode, while the remaining nodes on the host will continue to operate in client mode and automatically register with the new _Registrar_ in server node.

Node names must be unique within the same host. If a node attempts to register with a name that has already been registered by another node, the _Registrar_ will return the error `gen.ErrTaken`.

When a node attempts to establish a network connection with another node, it first sends a _resolve request_ (a UDP packet) to the _Registrar_, using the host name of the target node and port `4499`. In response to this request, the _Registrar_ returns a list of acceptors along with their parameters needed to establish the connection:

* The port number where the acceptor is running.
* The version of the _Handshake_ protocol.
* The version of the _Proto_ protocol.
* The _TLS flag_, which indicates whether TLS encryption is required for this connection.

Using these parameters, the node then establishes the network connection to the target node.

By default, the built-in network stack of Ergo Framework (`ergo.services/ergo/net`) is used for both incoming and outgoing connections. However, the node can work with multiple network stacks simultaneously (e.g., for compatibility with Erlang's network stack).

You can create multiple _acceptors_ with different sets of parameters for handling incoming connections. For outgoing connections, you can manage the connection parameters using the [Static Routes](static-routes.md) functionality, which allows you to control how connections are established with various nodes.
