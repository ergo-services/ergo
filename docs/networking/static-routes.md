---
description: Controlling outgoing connections with static routing
---

# Static Routes

When your code sends a message to a remote process, the framework needs to establish a connection to that node. But how does it know where the node is? By default, it asks the [Service Discovery](service-discovering.md) system (the Registrar) to look up the node's address. This works well for dynamic clusters where nodes come and go.

But sometimes you want more control. Maybe you know exactly where certain nodes are. Maybe you're behind a firewall and can't use dynamic discovery. Maybe you want to connect to external systems with fixed addresses. Static routes let you hardcode connection information directly, bypassing the discovery process entirely.

This isn't just about convenience. It's about control. When you define a static route, you're saying "I know better than the discovery system where this node is, and here's exactly how to reach it." The framework respects that - static routes are checked first, before any discovery queries.

## How It Works

The framework maintains an internal routing table. When you create an outgoing connection to a remote node, the framework:

1. **Checks static routes first** - Looks in the routing table for a match
2. **Falls back to discovery** - If no static route exists, queries the Registrar
3. **Tries proxy routes** - If direct connection fails, attempts proxy routes

This order is important. Static routes always win. If you've defined a route for `"prod-*"` that matches `prod-db@example.com`, the framework uses your route and never asks the Registrar. You've taken control.

The routing table uses pattern matching. When the framework needs to connect to `prod-db@example.com`, it checks all static routes against that name using Go's `regexp.MatchString`. Any routes whose patterns match become candidates. If multiple routes match, they're sorted by weight (higher weights first), and the framework tries them in order until one succeeds.

## Adding Static Routes

To add a static route, use `AddRoute` from the network interface:

```go
network := node.Network()

route := gen.NetworkRoute{
    Route: gen.Route{
        Host: "10.0.1.50",
        Port: 4370,
        TLS:  true,
    },
}

err := network.AddRoute("prod-db@example.com", route, 100)
if err != nil {
    // handle error
}
```

This tells the framework: "When connecting to `prod-db@example.com`, use host `10.0.1.50` on port `4370` with TLS enabled. This route has weight 100."

The **match pattern** is a regular expression. Exact names like `"prod-db@example.com"` match only that node. Patterns like `"prod-.*"` match multiple nodes - `prod-db@example.com`, `prod-api@example.com`, `prod-cache@example.com`. Use anchors (`^` and `$`) for precise matching: `"^prod-db@example.com$"` matches exactly that name and nothing else.

The **weight** determines priority when multiple routes match the same node. Higher numbers mean higher priority. If you have two routes for `"prod-.*"` - one with weight 100 (the default datacenter) and one with weight 200 (a faster backup datacenter) - the framework tries weight 200 first.

### Pattern Matching Examples

```go
// Exact match - only this specific node
network.AddRoute("database@prod", route1, 100)

// Prefix match - all production nodes
network.AddRoute("prod-.*", route2, 100)

// Suffix match - all nodes in a domain
network.AddRoute(".*@example.com", route3, 100)

// Complex pattern - production databases only
network.AddRoute("^prod-db[0-9]+@example.com$", route4, 100)
```

When the framework looks up `prod-db2@example.com`, it finds all matching routes: the prefix match (`prod-.*`), the suffix match (`.*@example.com`), and the complex pattern (`^prod-db[0-9]+@example.com$`). It sorts them by weight and tries the highest-weight route first.

## Route Configuration

The `gen.NetworkRoute` struct gives you fine-grained control over how connections are established:

### Direct Connection

The simplest route specifies connection parameters directly:

```go
route := gen.NetworkRoute{
    Route: gen.Route{
        Host: "192.168.1.100",
        Port: 4370,
        TLS:  true,
        HandshakeVersion: handshake.Version(), // optional, uses default if not set
        ProtoVersion:     proto.Version(),     // optional, uses default if not set
    },
}
```

When the framework uses this route, it connects to the specified host and port with TLS. The handshake and protocol versions default to the node's configured versions if you don't specify them explicitly.

### Route with Resolver

You can combine static patterns with dynamic resolution:

```go
route := gen.NetworkRoute{
    Resolver: registrar.Resolver(), // use specific registrar
    Route: gen.Route{
        Host: "custom.example.com", // override resolved host
        TLS:  true,                 // force TLS
    },
}

network.AddRoute("staging-.*", route, 100)
```

This hybrid approach uses the pattern to select which nodes use this route, then queries the resolver for connection details. The `Route` fields override any values returned by the resolver. In this example, even if the resolver returns a non-TLS route, the framework forces TLS. If the resolver returns `staging-db@internal` but you've specified `Host: "custom.example.com"`, the framework connects to your specified host instead.

Why would you do this? Imagine you have a staging environment behind a bastion host. The staging nodes register themselves in the discovery system with their internal addresses, but you need to connect through a specific gateway. The resolver pattern matches staging nodes, the resolver gets you the node's details, but your route configuration redirects the connection through your gateway.

### Custom Cookie

Each route can override the node's default authentication cookie:

```go
route := gen.NetworkRoute{
    Route: gen.Route{
        Host: "partner.external.com",
        Port: 4370,
    },
    Cookie: "shared-secret-with-partner",
}
```

This is essential when connecting to nodes outside your cluster. Your internal nodes use one cookie (say, `"internal-cluster-secret"`). An external partner's nodes use a different cookie (say, `"shared-secret-with-partner"`). Without per-route cookies, you'd have to use the same cookie everywhere or give up on connecting to external systems.

### Custom Certificates

For TLS connections, you can specify a custom certificate manager:

```go
customCert := node.CertManager() // or create a new one
route := gen.NetworkRoute{
    Route: gen.Route{
        Host: "secure.partner.com",
        Port: 4370,
        TLS:  true,
    },
    Cert:               customCert,
    InsecureSkipVerify: false, // enforce certificate validation
}
```

Different routes can use different certificates. Your production nodes might use certificates from one CA. A partner's nodes might use certificates from another CA. Each route gets its own certificate manager, allowing you to maintain separate trust chains.

Setting `InsecureSkipVerify: true` disables certificate validation. Use this only for testing or when connecting to nodes with self-signed certificates you trust but can't properly validate.

### Custom Network Flags

You can override network capabilities for specific routes:

```go
route := gen.NetworkRoute{
    Route: gen.Route{
        Host: "readonly.external.com",
        Port: 4370,
    },
    Flags: gen.NetworkFlags{
        Enable:                       true,
        EnableRemoteSpawn:            false, // don't let them spawn on us
        EnableRemoteApplicationStart: false, // don't let them start apps on us
        EnableImportantDelivery:      true,  // but do support important delivery
    },
}
```

This is about defense. When you connect to an external node, you probably don't want them spawning arbitrary processes on your node or starting applications remotely. Custom flags let you expose only the features you're comfortable with for that specific connection.

### Atom Mapping

Some advanced scenarios require translating atom values during communication:

```go
route := gen.NetworkRoute{
    Route: gen.Route{
        Host: "legacy.system.com",
        Port: 4370,
    },
    AtomMapping: map[gen.Atom]gen.Atom{
        "mynode@localhost":  "legacy_node",
        "process_manager":   "proc_mgr",
    },
}
```

When sending to this route, the framework automatically replaces `mynode@localhost` with `legacy_node` in all messages. On receiving, it reverses the mapping. This is rarely needed - most systems agree on naming conventions. But when integrating with legacy systems or systems with incompatible naming schemes, atom mapping saves you from rewriting every piece of code that references those atoms.

### Per-Route Logging

You can set the logging level for a specific connection:

```go
route := gen.NetworkRoute{
    Route: gen.Route{
        Host: "debug.target.com",
        Port: 4370,
    },
    LogLevel: gen.LogLevelTrace, // detailed logging for this route only
}
```

Normally your network stack runs at INFO or WARNING level. But when debugging a specific connection, you want TRACE logs for that connection without drowning in logs from all other connections. Per-route logging gives you surgical debugging.

## Multiple Routes and Failover

The framework tries routes in weight order when multiple patterns match the same node:

```go
// Primary datacenter - wider pattern
primaryRoute := gen.NetworkRoute{
    Route: gen.Route{Host: "10.0.1.50", Port: 4370, TLS: true},
}
network.AddRoute("^prod-db@.*", primaryRoute, 200)

// Backup datacenter - more specific pattern
backupRoute := gen.NetworkRoute{
    Route: gen.Route{Host: "10.0.2.50", Port: 4370, TLS: true},
}
network.AddRoute("prod-db@example.com", backupRoute, 100)
```

When connecting to `prod-db@example.com`, both patterns match. The framework sorts them by weight and tries weight-200 first. If that connection fails (host unreachable, handshake failure, timeout), it tries weight-100. This gives you automatic failover.

**Important limitation:** You can't add the same pattern twice. `AddRoute` returns `gen.ErrTaken` if the pattern already exists - the pattern is the routing table key. To achieve multi-route failover for a single node, you need different patterns that both match:

```go
// These are different patterns that match the same node
network.AddRoute("^prod-db@example.com$", primaryRoute, 200)  // exact match with anchors
network.AddRoute("prod-db@example.com", backupRoute, 100)     // substring match
```

Both patterns match `prod-db@example.com`, but they're different strings, so both can be added to the routing table.

Alternatively, use a resolver-based route. The resolver can return multiple addresses, and the framework tries them in order, letting the resolver handle failover logic.

## Querying Routes

To see if a route exists for a node:

```go
routes, err := network.Route("prod-db@example.com")
if err == gen.ErrNoRoute {
    // no static route defined
} else {
    // routes contains all matching routes, sorted by weight descending
    for i, route := range routes {
        fmt.Printf("Route %d: %s:%d\n", i+1, route.Route.Host, route.Route.Port)
    }
}
```

This queries the routing table without establishing a connection. You get back all routes whose patterns match the node name, sorted by weight. The highest-weight route is first - that's the one the framework would try first when actually connecting.

## Removing Routes

To remove a static route:

```go
err := network.RemoveRoute("prod-db@example.com")
if err == gen.ErrUnknown {
    // no such route existed
}
```

The pattern you pass to `RemoveRoute` must exactly match the pattern you used in `AddRoute`. It's not a regex match - it's a literal string key lookup in the routing table. If you added `"prod-.*"`, you must remove `"prod-.*"` exactly.

Removing a route doesn't affect existing connections. If you have an active connection to `prod-db@example.com` and you remove its static route, the connection stays alive. Removing a route only affects future connection attempts. The next time the framework needs to connect to that node, it won't find the static route and will fall back to discovery.

## Proxy Routes

Sometimes you can't connect directly to a node. Maybe it's behind a firewall. Maybe it's in a private network. Proxy routes let you connect through an intermediate node:

```go
proxyRoute := gen.NetworkProxyRoute{
    Route: gen.ProxyRoute{
        To:    "backend-db@internal.local",  // final destination
        Proxy: "gateway@dmz.example.com",    // intermediate node
    },
}

network.AddProxyRoute("backend-.*@internal.local", proxyRoute, 100)
```

When the framework needs to connect to `backend-db@internal.local`, it establishes a connection to `gateway@dmz.example.com` first, then asks the gateway to proxy the connection to the final destination. The gateway handles forwarding messages between you and the backend node.

Proxy routes have the same pattern matching and weight semantics as direct routes. You can define multiple proxy routes for the same pattern with different weights for failover.

### Proxy Configuration

```go
proxyRoute := gen.NetworkProxyRoute{
    Route: gen.ProxyRoute{
        To:    "target@backend",
        Proxy: "gateway@dmz",
    },
    Cookie: "gateway-specific-cookie",        // authenticate to gateway
    MaxHop: 3,                                // limit proxy chain depth
    Flags: gen.NetworkProxyFlags{
        Enable:                  true,
        EnableLink:              true,        // allow link operations through proxy
        EnableMonitor:           true,        // allow monitor operations
        EnableSpawn:             false,       // don't allow spawning through proxy
    },
}
```

`MaxHop` limits proxy chaining. If the gateway itself needs to proxy through another node, and that node proxies through yet another node, `MaxHop` prevents infinite loops. The default is 8. Each proxy hop decrements the counter. When it reaches zero, the framework refuses to proxy further.

The `Flags` control what operations the proxy allows. Maybe your gateway allows monitoring remote processes but doesn't allow spawning processes through the proxy. This gives you granular security control at the proxy level.

## Static Routes vs Discovery

Static routes are checked first, always. When the framework needs to connect to a node:

1. **Check routing table** - Pattern match against static routes
2. **Try static routes** - Attempt connection using matched routes (by weight order)
3. **Query discovery** - If no static route exists or all failed, ask the Registrar
4. **Try discovered routes** - Attempt connection using discovered addresses
5. **Try proxy discovery** - If direct connection fails, try discovered proxy routes
6. **Fail** - Return `gen.ErrNoRoute`

This priority order means static routes override discovery. If you have a static route for `prod-db` pointing to `10.0.1.50`, the framework never asks the Registrar for `prod-db`'s address. It just uses your route. This is by design - you're explicitly taking control.

But combining them is powerful. You can define static routes with resolvers:

```go
route := gen.NetworkRoute{
    Resolver: etcdRegistrar.Resolver(),
    Route: gen.Route{
        TLS: true,  // force TLS even if resolver says otherwise
    },
}
network.AddRoute("prod-.*", route, 100)
```

Now all production nodes use the static route for pattern matching, but the resolver for address lookup. You get the control of static routes (selecting which nodes use this configuration) with the dynamism of discovery (nodes can move without updating your code).

## When to Use Static Routes

**Fixed infrastructure** - If your nodes run on specific servers with static IPs, static routes are simpler than running a discovery service. Add routes for your database, cache, and API servers, and you're done.

**Firewall restrictions** - When discovery protocols can't traverse your firewall, static routes work around it. The internal nodes discover each other normally. External access uses static routes pointing to your gateway.

**External integration** - Connecting to nodes outside your cluster almost always requires static routes. You don't control their discovery system (if they even have one). You just need to reach specific addresses.

**Testing** - Hardcoding routes during development lets you point at local test nodes without configuring a full discovery system.

**Performance** - Static routes eliminate discovery latency. The framework connects immediately without the resolver round-trip. For frequently accessed nodes, this shaves milliseconds off connection establishment.

**Security boundaries** - Different routes can use different cookies and certificates. When integrating multiple trust domains, static routes let you configure each boundary explicitly.

Static routes aren't a replacement for discovery. They're a tool for cases where discovery doesn't fit. Most production clusters use discovery for internal nodes (dynamic, automatic) and static routes for fixed external connections (explicit, controlled). The framework supports both, and they work together.

For details on how connections are established, see [Network Stack](network-stack.md). For understanding the discovery system that static routes bypass, see [Service Discovery](service-discovering.md).
