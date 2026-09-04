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

This order is important, and step 2 is reached only when step 1 found **nothing**. A matching static route is not a preference, it is the whole answer: every matching route is tried by weight, and if they all fail the attempt ends with `gen.ErrNoRoute`. The registrar is never asked as a second chance. Defining a route for `"prod-*"` therefore takes `prod-db@example.com` out of discovery permanently - you have taken control, including of the failure.

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
    Resolver: registrar.Resolver(), // ask this registrar for these nodes
}

network.AddRoute("staging-.*", route, 100)
```

The pattern selects **which** nodes are resolved this way. It does not blend the two sources: once the resolver answers, the framework builds a fresh route out of what came back, and any `Route` fields configured beside the resolver contribute nothing to it. Host, port, TLS and the flags all come from the resolver. Only two things fall back to the node's own configuration when the resolver leaves them empty: the certificate manager, and the cookie.

So a `TLS: true` sitting next to a `Resolver` does not force TLS onto a resolver answer that says otherwise, and a `Host` there does not redirect the connection. Use this form to point a subset of nodes at a different discovery service. To dictate the address yourself, give the route a `Route` and no `Resolver` - a plain static route is the form that is honoured verbatim.

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
    Cert: customCert,
}
```

Different routes can use different certificates. Your production nodes might use certificates from one CA. A partner's nodes might use certificates from another CA. Each route gets its own certificate manager, allowing you to maintain separate trust chains.

Certificate **validation**, on the other hand, is not per route. `gen.NetworkRoute` carries an `InsecureSkipVerify` field, but every outgoing path overwrites it with the node-wide `NetworkOptions.InsecureSkipVerify` before dialling, so setting it on a route neither loosens nor tightens anything. One route cannot be strict while another is lax: the node decides, and the setting to reach for is `NetworkOptions.InsecureSkipVerify`. Incoming connections are the exception - there `AcceptorOptions.InsecureSkipVerify` is honoured per acceptor.

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

{% hint style="warning" %}
**Proxying is not implemented yet.** The API below exists and a proxy route can be registered and read back, but no connection is ever made through it: every proxy path ends in `connectProxy`, which logs "proxy feature is not implemented yet" and returns `gen.ErrUnsupported`. A node that can only be reached through a gateway cannot be reached at all today. This section describes the shape the feature will take; treat it as a preview, not as something to build on.
{% endhint %}

Sometimes you can't connect directly to a node. Maybe it's behind a firewall. Maybe it's in a private network. Proxy routes are meant to let you connect through an intermediate node:

```go
proxyRoute := gen.NetworkProxyRoute{
    Route: gen.ProxyRoute{
        To:    "backend-db@internal.local",  // final destination
        Proxy: "gateway@dmz.example.com",    // intermediate node
    },
}

network.AddProxyRoute("backend-.*@internal.local", proxyRoute, 100)
```

The intent is that connecting to `backend-db@internal.local` opens a connection to `gateway@dmz.example.com` first and asks the gateway to forward. Today the attempt stops at the gateway step with `gen.ErrUnsupported`.

Proxy routes have the same pattern matching and weight semantics as direct routes. You can define multiple proxy routes for the same pattern with different weights for failover.

### Proxy Configuration

```go
proxyRoute := gen.NetworkProxyRoute{
    Route: gen.ProxyRoute{
        To:    "target@backend",
        Proxy: "gateway@dmz",
    },
    Cookie: "gateway-specific-cookie",        // authenticate to gateway
    MaxHop: 3,                                // intended chain-depth limit
    Flags: gen.NetworkProxyFlags{
        Enable:                       true,
        EnableRemoteSpawn:            false,
        EnableRemoteApplicationStart: false,
        EnableEncryption:             true,
        EnableImportantDelivery:      true,
    },
}
```

Those five are the whole of `gen.NetworkProxyFlags`. There is no `EnableLink` or `EnableMonitor` field, and `EnableSpawn` is a method on `gen.Network`, not a flag.

`MaxHop` is stored and reported but not yet acted on: nothing decrements it, and there is no `DefaultProxyMaxHop` constant. It is part of the same unimplemented feature as the rest of this section.

## Static Routes vs Discovery

Static routes are checked first, always. When the framework needs to connect to a node:

1. **Check routing table** - Pattern match against static routes
2. **Try static routes** - Attempt connection using matched routes (by weight order). If any matched, this is the last step: on failure the answer is `gen.ErrNoRoute`
3. **Query discovery** - Only when **no** static route matched, ask the Registrar
4. **Try discovered routes** - Attempt connection using discovered addresses
5. **Try proxy discovery** - If direct connection fails, try discovered proxy routes (which currently end in `gen.ErrUnsupported`, see the warning above)
6. **Fail** - Return `gen.ErrNoRoute`

A static route is not a preference the framework may reconsider. If you have one for `prod-db` pointing to `10.0.1.50` and that address is down, the connection fails - the Registrar is never asked, even though it might know a working address. This is by design: you took control, and that includes the failure. Remove or narrow the route to hand the node back to discovery.

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
