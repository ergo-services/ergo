---
description: Web UI and MCP surface for monitoring, inspecting and managing Ergo nodes
---

# Observer

Observer is an application that embeds into your node and opens it up for inspection. One HTTP listener serves three things: the web UI built into the binary, the API that UI runs on, and an MCP surface for an AI agent. All three read the same node through the built-in `system` application, and all three are bounded by the same authorization, apart from two deliberately open paths noted below.

For the tour of the UI, see [Inspecting With Observer](../../advanced/observer.md). For working through an agent, see [Inspecting With an AI Agent](../../advanced/mcp.md). This page is how you add it and what you can configure.

## Adding Observer to a node

<pre class="language-go"><code class="lang-go">import (
	"ergo.services/ergo"
	"ergo.services/application/observer"
	"ergo.services/ergo/gen"
)

func main() {
	options := gen.NodeOptions{
		Applications: []gen.ApplicationBehavior{
			observer.CreateApp(observer.Options{}),
		},
	}
	node, err := ergo.StartNode("mynode@localhost", options)
	if err != nil {
		panic(err)
	}
	node.Wait()
}
</code></pre>

Open `http://localhost:9911` in your browser, and point an MCP client at `http://localhost:9911/mcp`.

Nothing has to be deployed on the nodes you inspect. Observer talks to the `system` application that every Ergo node runs, so one node with Observer reaches the whole cluster.

## Surfaces

A listener serves three surfaces:

| Surface | Path | What it is |
| ------- | ---- | ---------- |
| UI | `/` | The bundle built into the binary. Only one listener may serve it. |
| API | `/sse`, `/api/*` | What the bundle calls: the event stream and the request endpoints. |
| MCP | `/mcp` | The agent-facing surface: `ergo://` resources and tools. |

Two paths under `/api/` are deliberately open, because both have to work before a caller is authenticated: `GET /api/capabilities`, which is how a client discovers what this endpoint offers, and `POST /api/enroll`, which is the exchange that authenticates it in the first place. Both are rate-limited, `enroll` on its own hard limit, and neither reveals anything about the node. Everything else under `/api/` goes through the authorizer.

The default configuration serves all three surfaces on `localhost:9911`. That is the right shape for a development machine and the wrong one for a deployment other people can reach, which is what the rest of this page is about.

## Options

`observer.CreateApp` accepts `observer.Options`:

| Option | Default | Meaning |
| ------ | ------- | ------- |
| `Host` | `localhost` | Interface to bind. Belongs to a `Listener` once `Listeners` is set. |
| `Port` | `9911` | Port to bind. Belongs to a `Listener` once `Listeners` is set. |
| `PoolSize` | `25` | Workers handling POST requests. |
| `Ceiling` | zero, everything permitted | The limit of the whole deployment. A listener, and then a caller, can only be given less. |
| `Authorizer` | none, the listener is open | Identifies the caller. Belongs to a `Listener` once `Listeners` is set. |
| `RateLimit` | `0`, no limit | Requests per second one caller may make. Belongs to a `Listener` once `Listeners` is set. |
| `AllowedOrigins` | see [Origins](#origins) | Browser origins allowed on top of `DefaultAllowedOrigins`. Belongs to a `Listener` once `Listeners` is set. |
| `Listeners` | one listener from `Host:Port` | Runs one endpoint per entry, each with its own authorization. `Host` and `Port` must then be left unset. |
| `Enrollment` | empty, `/api/enroll` is not served | The one-time secret the cloud presents to confirm this endpoint is this observer. |
| `JobMaxRetention` | `5m` | The longest a finished cluster run may keep its result. |
| `JobLimit` | `32` | How many runs the observer holds at once, over every caller. |
| `ClusterLens` | see [Cluster lens](#cluster-lens) | The map of the cluster and the watchers keeping it current. |
| `LogLevel` | the node's | Log level of the observer's own processes. |

The single-listener fields (`Host`, `Port`, `Authorizer`, `RateLimit`, `AllowedOrigins`) are a shorthand for one entry in `Listeners`. Setting any of them **together with** `Listeners` is refused at start, with a message naming those five - they are not silently ignored, and not merged.

## Listeners

One entry per endpoint. This is how the same node offers a full-access UI to an operator on loopback and a read-only API to something else, without either being able to reach the other's policy:

<pre class="language-go"><code class="lang-go">observer.CreateApp(observer.Options{
	Ceiling: observer.Ceiling{Deny: []string{"manage.kill"}},

	Listeners: []observer.Listener{
		{
			Name: "local",
			Port: 9911,
		},
		{
			Name:           "public",
			Port:           9912,
			Ceiling:        observer.Ceiling{ReadOnly: true},
			UI:             observer.SurfaceUI{Disable: true},
			MCP:            observer.SurfaceMCP{Disable: true},
			AllowedOrigins: []string{"https://ops.example.com"},
			RateLimit:      50,
		},
	},
})
</code></pre>

| Field | Default | Meaning |
| ----- | ------- | ------- |
| `Name` | `:<port>` | Goes into the start log and into `HandleInspect`. |
| `Host` | `localhost` | Interface to bind. |
| `Port` | required | Port to bind, unique among the listeners. |
| `CertManager` | nil, plain HTTP | Serves this listener over TLS. See [CertManager](../../basics/certmanager.md). |
| `UI` | served | `SurfaceUI{Disable: bool}`. At most one listener may serve the UI. |
| `API` | served | `SurfaceAPI{Ceiling *Ceiling}`. No switch of its own: `/sse` and `/api/*` are what a listener is for. |
| `MCP` | served | `SurfaceMCP{Disable, Ceiling, Instructions, CacheTTL}`, see [The MCP surface](#the-mcp-surface). |
| `Authorizer` | none, the listener is open | Identifies the caller arriving here. |
| `Ceiling` | zero | Narrows the deployment ceiling for everyone arriving here. |
| `RateLimit` | `0`, no limit | Requests per second per caller: by subject when there is an authorizer, by address otherwise. The static bundle is not metered. |
| `MaxStreams` | `64` | Streams open here at once, `/sse` and `/mcp` together. |
| `MaxSubscriptions` | `128` | Live subscriptions one stream may hold. It also sizes the stream's mailbox. |
| `AllowedOrigins` | see [Origins](#origins) | Browser origins allowed here, on top of `DefaultAllowedOrigins`. |

Zero means the default for `MaxStreams` and `MaxSubscriptions`, not "no limit". For `RateLimit` zero does mean no limit: a missing rate limit is safe, a missing stream limit is not.

The start log states what each listener ended up serving, which is worth reading once after a configuration change:

```
listener "local" localhost:9911 surfaces=api,ui,mcp authorizer=no ceiling=narrowed origins=5 ratelimit=0
listener "public" localhost:9912 surfaces=api authorizer=no ceiling=read-only origins=6 ratelimit=50
```

The local listener reads `narrowed` rather than `full` because the deployment `Ceiling` above denies `manage.kill`, and a deployment ceiling applies to every listener. `full` appears only when nothing above the listener has narrowed anything.

## Ceilings

A ceiling is the limit of what a caller may ask for, written as capability names. Every level carries one and every level can only narrow: the deployment ceiling bounds the listener, the listener bounds the surface, and the authorizer bounds the caller.

<pre class="language-go"><code class="lang-go">observer.Ceiling{
	ReadOnly: true,                               // refuse every mutating capability
	Allow:    []string{"inspect.process_list"},   // unset does not narrow; empty permits nothing
	Deny:     []string{"manage.kill"},            // wins over Allow
	Nodes:    []string{"orders@host"},            // unset does not narrow; empty permits no node
}
</code></pre>

Capability names come in two planes. Everything under `inspect.` reads, everything under `manage.` changes the node, and `ReadOnly` is exactly "refuse the `manage.` plane".

`inspect.` — `capabilities`, `node`, `node_short`, `network`, `connection`, `connection_list`, `process_list`, `process_range`, `process`, `process_state`, `process_lookup`, `meta`, `meta_state`, `app_tree`, `subtree`, `application_list`, `event_list`, `event`, `event_stream`, `log`, `tracing`, `goroutines`, `heap_profile`, `types`, `errors`, `atoms`, `cron_info`, `cron_schedule`, `registrar_nodes`, `registrar_routes`, `registrar_proxy_routes`, `registrar_application_routes`

`manage.` — `send`, `send_meta`, `send_exit`, `send_exit_meta`, `kill`, `set_log_level`, `set_process_log_level`, `set_meta_log_level`, `set_node_tracing_sampler`, `set_process_tracing_sampler`, `set_process_send_priority`, `set_process_compression`, `set_process_compression_type`, `set_process_compression_level`, `set_process_compression_threshold`, `set_process_keep_network_order`, `set_process_important_delivery`, `set_meta_send_priority`, `app_start`, `app_stop`, `app_unload`

Two details of `Allow` and `Nodes` decide what an empty slice means. Unset (`nil`) does not narrow. Present and empty permits nothing. That distinction is what makes composition work: narrowing two allowlists intersects them, and two lists with nothing in common intersect to empty rather than to "no restriction".

`Narrow` is the composition the observer applies at every level, and it is exported if you compose ceilings yourself:

<pre class="language-go"><code class="lang-go">import "ergo.services/application/observer/access"

// never wider than either argument: ReadOnly spreads, Deny accumulates,
// non-empty lists intersect
bounded := access.Narrow(deployment, perCaller)
</code></pre>

The UI reads its own ceiling and hides what it cannot use, so an operator on a read-only listener sees no kill buttons rather than buttons that fail.

## Authorizers

Without an authorizer a listener is open: everyone who reaches it gets the listener's ceiling. That is fine for `localhost` on a development machine and nothing else.

An authorizer answers who the caller is:

<pre class="language-go"><code class="lang-go">type Authorizer interface {
	Authorize(request *http.Request) (Identity, error)
}

type Identity struct {
	Subject string    // a user id, a service account, a token subject
	Tenant  string    // groups subjects that share a scope; empty when the deployment has one
	Ceiling Ceiling   // narrows the listener's ceiling for this caller
}
</code></pre>

It runs on the web server goroutine, before the request reaches an actor, so it must not block for long. Returning `access.ErrUnauthenticated` answers 401, `access.ErrForbidden` answers 403, and any other error answers 403. No detail from the error reaches the caller.

`Subject` is more than a label: it scopes what belongs to whom. A cluster run started by one caller is readable and cancellable only by that caller, and a keyed resource cursor belongs to the caller that asked for it. Ownership is the tenant and the subject together, so two callers share a run only when both fields match - the same subject under a different tenant is a different owner.

Observer ships one implementation, for the common deployment where a proxy in front has already authenticated the request:

<pre class="language-go"><code class="lang-go">import "ergo.services/application/observer/access"

observer.Listener{
	Name: "team",
	Host: "127.0.0.1",           // only the proxy can reach it
	Port: 9913,
	Authorizer: access.TrustedHeader{
		Subject: "X-Auth-Request-Email",
		Tenant:  "X-Auth-Request-Domain",
		Groups:  "X-Auth-Request-Groups",
		Ceilings: map[string]observer.Ceiling{
			"viewer": {ReadOnly: true, Deny: []string{"manage.kill"}},
			"sre":    {Deny: []string{"manage.kill"}},
		},
	},
}
</code></pre>

`TrustedHeader` verifies nothing. Its whole security is that nothing but the proxy can reach the listener, which is why the node refuses to start when such a listener binds a non-loopback address. State `ReachableOnlyByProxy: true` if the path is restricted some other way, and own that claim.

A caller in several listed groups gets the wider of their ceilings, and that is why the groups must be **comparable**: the node refuses to start if two of them are ceilings where neither contains the other, since a caller holding both would end up with more than either grants. That is the reason `viewer` above repeats the `manage.kill` deny it already gets from `ReadOnly` - without it, read-only and "everything but kill" are two different shapes rather than one inside the other. A caller in no listed group is refused.

## Origins

A browser sends an `Origin` header, and the observer refuses a request from an origin it does not allow. The page a listener served itself is always allowed, so a same-origin bundle needs no configuration.

`DefaultAllowedOrigins` is added to every listener:

<pre class="language-go"><code class="lang-go">var DefaultAllowedOrigins = []string{
	"http://localhost:*",   // any port, so a dev server reaches it
	"http://127.0.0.1:*",
	"http://[::1]:*",
	"https://ergo.observer",
	"https://app.ergo.observer",
}
</code></pre>

Set it to `nil` before starting the application to allow nothing but what each listener names. Per-listener entries are added to it, not put in its place.

Each entry is `scheme://host[:port]` with no path. A port of `*` matches any port, a leftmost-label wildcard (`https://*.example.com`) matches one label and not the parent, and `*` alone means any origin without credentials. Anything else fails the start rather than silently blocking every cross-origin call.

Behind a proxy that terminates TLS the page comes back over `https` while the request reaches the observer as plain `http`. The observer reads `X-Forwarded-Proto` for exactly that, so a bundle served by an ingress under its own hostname is treated as same-origin without that hostname being configured anywhere.

## The MCP surface

`SurfaceMCP` configures what an agent gets:

| Field | Default | Meaning |
| ----- | ------- | ------- |
| `Disable` | served | Stops serving `/mcp` on this listener. |
| `Ceiling` | the listener's | Narrows the listener ceiling for this surface alone. |
| `Instructions` | none | What an agent is told about this cluster before it asks anything. |
| `CacheTTL` | `5m` | How long a client may keep the tool and resource listings, and the discovery answer. |

`Ceiling` separates surfaces, not callers: whoever reaches this listener reaches both surfaces, so narrow `API` as well or put the surfaces on listeners of their own.

`Instructions` is the one place to say what no amount of inspection reveals: which node runs which part of the business, where a flow begins, what not to touch. It is added to the guidance the observer gives about itself rather than replacing it, so navigating the surface stays described whatever you write:

<pre class="language-go"><code class="lang-go">MCP: observer.SurfaceMCP{
	Instructions: "Orders begin at orders@* and settle through billing@*. " +
		"The nodes named archive@* are cold storage: read them, never touch them.",
},
</code></pre>

`CacheTTL` matters during development and nowhere else. Inside one process none of those listings change, so a long value costs nothing in a running deployment. It costs when the binary is rebuilt: a client outlives the restart, and until the TTL expires it calls tools with the arguments of the binary it first met. Set it to a few seconds where you rebuild often.

## Cluster lens

The cluster map is the observer's own view of every node it can see, kept current by watching them. It is what an agent reads as `ergo://cluster`.

| Option | Default | Meaning |
| ------ | ------- | ------- |
| `WatchLimit` | `5000` | Nodes being watched. Nodes discovered beyond it stay on the map without data. |
| `Concurrency` | `64` | Nodes being connected at once. |
| `WatchPeriod` | `3s` | Interval between the snapshots a watched node publishes. Larger clusters want a larger value: every node sends one snapshot per period. |
| `ReconcilePeriod` | `1m` | Interval between the passes that check the membership bookkeeping. Nodes are dropped as they run out of support, not on this timer, so it is a backstop and wants a large value. |
| `GracePeriod` | `1m` | How long the last known peer list of an unreachable node still counts as evidence that its peers exist. Until it expires, a network split does not erase the far side of the map. |
| `LastReadingPeriod` | `GracePeriod` | How long the last reading of a node that went away stays on the map, marked stale. |

## Enrollment

`Enrollment` serves `POST /api/enroll`, a one-time exchange that lets the cloud confirm the endpoint it was given really is this observer:

<pre class="language-go"><code class="lang-go">Enrollment: observer.EnrollmentOptions{
	Token:     os.Getenv("OBSERVER_ENROLL_TOKEN"),
	ClusterID: "orders-prod",
}
</code></pre>

The token burns on the first success and the endpoint answers 410 after that. A wrong token is a 403, counted in the manager's `HandleInspect`, and rate limited on its own. An empty `Token` means the endpoint is not served at all.

## What it costs the node

Observer is a normal application on the node it runs on, and most of what it does is cheap: reading counters the framework already maintains. Three things are not.

A goroutine dump and a heap profile stop the world for as long as the walk takes, on the node being inspected. They are worth asking for, and worth asking for once.

An open stream is not free: each `/sse` or `/mcp` stream holds a goroutine and an actor for its whole life, which is what `MaxStreams` bounds, and an `/sse` stream also holds a gzip writer, since the UI stream is compressed and the agent stream is not. Subscriptions inside a stream cost a producer on the observed node, which is what `MaxSubscriptions` bounds.

A cluster run holds a pool of workers until it finishes or expires, which is what `JobLimit` and `JobMaxRetention` bound. Those limits exist so that one careless client cannot leave work behind on the node.

## Inspecting the observer itself

Every observer process implements `HandleInspect`, so the observer is visible in the observer. The web process reports its listener name, address, TLS, surfaces, authorizer, ceiling, origins and how many were refused, rate limit, open streams against the limit, refusals by status code, whether enrollment is configured, and uptime. Read it from the UI, from another process with `Inspect(pid)`, or with the `process_state` tool over MCP.

## Deployment recipes

**A development machine.** `observer.Options{}`. Loopback, no authorizer, everything permitted, all three surfaces. Nothing else is needed, and nothing else is safe.

**Behind an authenticating proxy.** One listener on `127.0.0.1` with `access.TrustedHeader` and per-group ceilings, the proxy in front of it, and a deployment ceiling denying whatever nobody should have. The UI, the API and MCP all inherit the caller's identity, so an operator and their agent are bounded the same way.

**A cloud-facing read-only endpoint.** A second listener with `Ceiling{ReadOnly: true}`, `UI` disabled because the cloud serves its own bundle, `AllowedOrigins` naming that bundle's origin, and a rate limit. The first listener keeps serving the local UI at full access.

## See also

- [Inspecting With Observer](../../advanced/observer.md) - the tour of the web UI
- [Inspecting With an AI Agent](../../advanced/mcp.md) - the MCP surface in practice
- [Inspecting Actor State](../../advanced/inspecting-state.md) - what your own actors expose to all of this
- [CertManager](../../basics/certmanager.md) - serving a listener over TLS
