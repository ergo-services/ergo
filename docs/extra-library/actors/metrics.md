# Metrics

The metrics actor collects runtime statistics from an Ergo node and exposes them as a Prometheus HTTP endpoint. It runs as a regular process: spawn it, and it starts serving `/metrics` with node, network, process, and event telemetry.

For application-specific metrics (request rates, business counters), you extend the actor with custom Prometheus collectors.

## Why Monitor Actors

Actor systems are dynamic. Processes spawn and terminate constantly, messages flow through mailboxes asynchronously, and load depends on message routing and supervision trees. Traditional monitoring (thread pools, request queues) does not capture this. The metrics actor tracks process lifecycle, mailbox pressure, message throughput, event fanout, network traffic, and delivery errors, giving visibility into what the actor runtime is actually doing.

## ActorBehavior Interface

```go
type ActorBehavior interface {
    gen.ProcessBehavior

    Init(args ...any) (Options, error)

    HandleMessage(from gen.PID, message any) error
    HandleCall(from gen.PID, ref gen.Ref, message any) (any, error)
    HandleEvent(event gen.MessageEvent) error
    HandleInspect(from gen.PID, item ...string) map[string]string

    CollectMetrics() error
    Terminate(reason error)
}
```

Only `Init()` is required. All other callbacks have default implementations.

Two patterns for custom metrics:

**Periodic collection:** implement `CollectMetrics()` to query state at intervals. Use when metrics reflect current state from other actors or external sources.

**Event-driven updates:** implement `HandleMessage()` or `HandleEvent()` to update metrics as events occur. Use when your application produces natural event streams.

## Basic Usage

```go
package main

import (
    "ergo.services/actor/metrics"
    "ergo.services/ergo"
    "ergo.services/ergo/gen"
)

func main() {
    node, _ := ergo.StartNode("mynode@localhost", gen.NodeOptions{})
    defer node.Stop()

    // Required. ProcessInit refuses to start on a node that cannot decode the
    // registration and update messages this actor accepts.
    node.Network().RegisterTypes(metrics.NetworkTypes())
    node.Network().RegisterErrors(metrics.ErrorTypes())

    node.Spawn(metrics.Factory, gen.ProcessOptions{}, metrics.Options{})

    // Metrics available at http://localhost:3000/metrics
    node.Wait()
}
```

Registering the wire types is the caller's job, not the library's. Spawn the actor on a node that does not know them and `ProcessInit` fails with "metrics.MetricType is not registered on this node". Besides registering on the node as above, they can be declared on the application that hosts the actor, in `gen.ApplicationSpec.Network` - `RegisterTypes: metrics.NetworkTypes()` and `RegisterErrors: metrics.ErrorTypes()` - which is processed before any of its processes spawn.

If you run the metrics actor through [radar](../applications/radar.md) instead of spawning it yourself, this is already handled: radar declares both sets in its own `ApplicationSpec.Network`, and application load runs before any of its processes spawn.

Default configuration:
- **Host**: `localhost` - standalone mode only
- **Port**: `3000` - standalone mode only
- **Path**: `/metrics`
- **CollectInterval**: `10 seconds`
- **TopN**: `50`

`Host` and `Port` are defaulted **only in standalone mode**, which is when `Options.Shared` is nil. In shared mode the primary still starts the HTTP server and passes `Host` through as given - so an empty `Host` there is not `localhost`, it is every interface. Set it explicitly on a shared-mode primary unless exposing the endpoint publicly is what you want. `Path`, `CollectInterval` and `TopN` are defaulted in both modes.

## Configuration

```go
options := metrics.Options{
    Host:            "0.0.0.0",        // Listen on all interfaces
    Port:            9090,              // HTTP port
    Path:            "/metrics",       // HTTP path
    CollectInterval: 5 * time.Second,  // Collection frequency
    TopN:            50,               // Top-N entries per metric group
}

node.Spawn(metrics.Factory, gen.ProcessOptions{}, options)
```

**Host** determines which interface the HTTP server binds to. Use `"localhost"` for development, `"0.0.0.0"` for production/containers.

**Port** should not conflict with other services. Prometheus conventionally uses `9090`, Observer UI defaults to `9911`.

**TopN** controls how many top entries are tracked for each metric group (mailbox depth, utilization, latency for processes; subscribers, published, deliveries for events). Higher values increase Prometheus cardinality.

**CollectInterval** controls how frequently the actor queries node statistics. Collecting more frequently than your Prometheus scrape interval wastes resources.

**Mux** accepts an external `*http.ServeMux`. The metrics actor registers its handler on this mux and skips starting its own HTTP server. Useful for serving metrics alongside other handlers on a single port:

```go
mux := http.NewServeMux()

metricsOpts := metrics.Options{
    Mux:             mux,
    CollectInterval: 5 * time.Second,
}
node.Spawn(metrics.Factory, gen.ProcessOptions{}, metricsOpts)

healthOpts := health.Options{Mux: mux}
node.SpawnRegister("health", health.Factory, gen.ProcessOptions{}, healthOpts)
```

When `Mux` is set, `Host` and `Port` are ignored.

## Base Metrics

The actor automatically collects metrics without any configuration. All metrics carry a `node` label identifying the source node.

### Node Metrics

Uptime, process counts (total, running, zombie), spawn/termination counters, memory (OS used, runtime allocated), CPU time (user, system), application counts, registered names/aliases/events, event publish/receive/delivery counters, and Send/Call delivery error counters (local and remote).

Delivery errors are split by type: `ergo_send_errors_local_total` and `ergo_call_errors_local_total` count failures where the target process is unknown, terminated, or has a full mailbox. `ergo_send_errors_remote_total` and `ergo_call_errors_remote_total` count connection failures to remote nodes.

### Log Metrics

Log message count by level (`trace`, `debug`, `info`, `warning`, `error`, `panic`). Counted once before fan-out to loggers.

### Network Metrics

Connected node count, per-node uptime, message and byte rates (in/out per remote node), cumulative connections established/lost, and per-acceptor handshake error count. Fragmentation metrics per remote node: fragments sent/received, fragmented messages sent/reassembled, assembly timeouts. Compression metrics per remote node: compressed messages sent, bytes before/after compression, decompressed messages received, bytes before/after decompression. Compression ratio (`original / compressed`) reveals whether compression is effective for each connection.

### Mailbox Latency Metrics

Requires building with `-tags=latency`. Measures how long the oldest message has been waiting in each process's mailbox. Provides distribution across ranges (1ms to 60s+), max latency, and top-N processes by latency.

### Mailbox Depth Metrics

Always active. Counts messages queued in each process's mailbox. Distribution across ranges (1 to 10K+), max depth, and top-N processes by depth. Complementary to latency: depth is "how many messages are waiting", latency is "how long the oldest has been waiting".

### Process Metrics

Always active. Includes:

- **Utilization:** ratio of callback running time to uptime. Distribution, max, and top-N.
- **Init time:** ProcessInit duration. Max and top-N.
- **Throughput:** messages in/out per process (top-N) and node-level aggregates.
- **Wakeups and drains:** wakeup count and drain ratio (messages processed per wakeup). Drain ratio distinguishes between slow callbacks (drain ~1) and high-throughput batching (drain ~100) at the same utilization level.
- **Liveness:** detects processes stuck in blocking calls. Computed as `RunningTime / (Uptime * MailboxLatency)`. A healthy process has RunningTime growing with activity (high score). A process blocked in a mutex, channel, or IO has RunningTime frozen while uptime and latency keep growing (score drops over time). Zombie processes are excluded (detected separately). Bottom-N surfaces the most stuck processes. Requires `-tags=latency`.

### Event Metrics

Always active. Per-event subscriber count, publish/delivery counts, and utilization state (`active`, `on_demand`, `idle`, `no_subscribers`, `no_publishing`). See [Events](../../basics/events.md) for the pub/sub model and [Pub/Sub Internals](../../advanced/pub-sub-internals.md) for the shared subscription optimization that affects delivery counters.

For the complete list of metric names, types, labels, and descriptions, see the [metrics actor README](https://github.com/ergo-services/actor).

## Custom Metrics

All custom metrics automatically receive a `node` const label. Do not include `"node"` in your variable label names.

### Helper Functions

Any actor on the same node can register and update custom metrics without importing `prometheus` or embedding the metrics actor:

```go
// Register metrics (sync Call, returns error)
metrics.RegisterGauge(w, "metrics_actor", "db_connections", "Active connections", []string{"pool"})
metrics.RegisterCounter(w, "metrics_actor", "cache_ops", "Cache operations", []string{"op"})
metrics.RegisterHistogram(w, "metrics_actor", "request_seconds", "Latency", []string{"path"}, nil)

// Update metrics (async Send)
metrics.GaugeSet(w, "metrics_actor", "db_connections", 42, []string{"primary"})
metrics.CounterAdd(w, "metrics_actor", "cache_ops", 1, []string{"hit"})
metrics.HistogramObserve(w, "metrics_actor", "request_seconds", 0.023, []string{"/api"})

// Remove a metric (async Send)
metrics.Unregister(w, "metrics_actor", "db_connections")
```

When the registering process terminates, the metrics actor automatically unregisters all metrics it owned.

### Embedding metrics.Actor

For direct access to the Prometheus registry or periodic collection via `CollectMetrics`:

```go
type AppMetrics struct {
    metrics.Actor

    activeUsers prometheus.Gauge
    registered  bool
}

func (m *AppMetrics) Init(args ...any) (metrics.Options, error) {
    m.activeUsers = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "myapp_active_users",
        Help: "Current number of active users",
    })

    return metrics.Options{
        Port:            9090,
        CollectInterval: 5 * time.Second,
    }, nil
}

func (m *AppMetrics) CollectMetrics() error {
    if m.registered == false {
        if err := m.Registry().Register(m.activeUsers); err != nil {
            return err
        }
        m.registered = true
    }

    count, err := m.Call(userService, getActiveUsersMessage{})
    if err != nil {
        m.Log().Warning("failed to get user count: %s", err)
        return nil
    }
    m.activeUsers.Set(float64(count.(int)))
    return nil
}
```

`Registry()` returns nil until `Init` has returned. The actor builds the registry afterwards, because whether it is a private registry or a `Shared` one is decided by the `Options` that `Init` hands back - so `m.Registry().MustRegister(...)` inside `Init` panics on a nil pointer. Build the collectors in `Init` and register them on the first `CollectMetrics`, which runs later on the actor's own goroutine.

For event-driven updates, implement `HandleMessage()` instead of `CollectMetrics()`:

```go
func (m *AppMetrics) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case requestCompletedMessage:
        m.requestsTotal.Inc()
        m.requestLatency.Observe(msg.duration.Seconds())
    }
    return nil
}
```

### Custom Top-N Metrics

Top-N metrics track the N highest (or lowest) values observed during each collection cycle. Unlike gauges or counters, a top-N metric accumulates observations and periodically flushes only the top entries to Prometheus as a GaugeVec. This is useful when you want to identify the most active, slowest, or largest items out of many, without creating a separate time series for each one.

Each top-N metric is managed by a dedicated actor spawned under a SimpleOneForOne supervisor. Registration creates this actor; observations are sent to it asynchronously. On each flush interval the actor writes the current top-N entries to Prometheus and resets for the next cycle.

```go
// Register a top-N metric (sync Call, returns error)
// TopNMax keeps the N largest values; TopNMin keeps the N smallest
metrics.RegisterTopN(w, "topn_supervisor_name", "slowest_queries", "Slowest DB queries",
    10, metrics.TopNMax, []string{"query", "table"})

// Observe values (async Send)
metrics.TopNObserve(w, gen.Atom("radar_topn_slowest_queries"), 0.250, []string{"SELECT ...", "users"})
metrics.TopNObserve(w, gen.Atom("radar_topn_slowest_queries"), 1.100, []string{"JOIN ...", "orders"})
```

The `to` parameter in `RegisterTopN` is the name of the supervisor managing top-N actors. The `to` parameter in `TopNObserve` is the actor name, by convention `"radar_topn_" + metricName`.

Ordering modes:

- `metrics.TopNMax`: keeps the N largest values (e.g., slowest queries, busiest actors, highest memory usage)
- `metrics.TopNMin`: keeps the N smallest values (e.g., lowest latency, least active processes)

When the process that registered a top-N metric terminates, the actor automatically cleans up and unregisters its GaugeVec from Prometheus.

When used through the [Radar](../applications/radar.md) application, the supervisor is already wired in and you use `radar.RegisterTopN` / `radar.TopNObserve` helpers instead.

### Shared Mode

A single metrics actor processes messages sequentially. Under high throughput, its mailbox becomes a bottleneck. Shared mode lets multiple metrics actor instances share the same Prometheus registry:

```go
shared := metrics.NewShared()

// Primary actor: owns HTTP endpoint and base metrics
primaryOpts := metrics.Options{
    Port:   9090,
    Shared: shared,
}

// Worker actors: handle custom metric updates only
workerOpts := metrics.Options{
    Shared: shared,
}
```

The primary actor starts the HTTP server and collects base metrics. Workers only process custom metric messages. All actors write to the same registry through the shared object. Works well with `act.Pool` for automatic load distribution.

## Integration with Prometheus

```yaml
scrape_configs:
  - job_name: 'ergo-nodes'
    static_configs:
      - targets:
          - 'localhost:3000'
          - 'node1.example.com:3000'
    scrape_interval: 15s
```

For dynamic discovery in Kubernetes, use Prometheus service discovery instead of static targets.

## Grafana Dashboard

The metrics package includes a pre-built Grafana dashboard (`ergo-cluster.json`) for monitoring Ergo clusters.

Import it in Grafana: Dashboards > Import > upload `ergo-cluster.json` > select your Prometheus data source. The `$node` dropdown at the top filters all panels by selected nodes.

The dashboard is organized top-down: Summary row at the top for cluster health at a glance, then Mailbox Latency and Depth for backpressure analysis, then collapsed rows for Events, Process Activity, Processes, Resources, Logging, and Network. The Network row includes compression overview (ratio, rate, percentage), per-node compression ratio, fragmentation rates (cluster and per-node), connectivity strength, and connection events. Each row focuses on a specific aspect of cluster behavior and can be expanded when investigating issues.

For detailed panel descriptions, see the [metrics actor README](https://github.com/ergo-services/actor).

## Observer Integration

The metrics actor integrates with Observer via `HandleInspect()`. Inspecting the process shows total metric count, HTTP endpoint, collection interval, and current values for all metrics.

When embedding `metrics.Actor` and overriding `HandleInspect()`, your keys are merged on top of base inspection data.

## Radar Application

If your node needs both Prometheus metrics and Kubernetes health probes, consider the [Radar](../applications/radar.md) application. It runs the metrics actor and [Health](health.md) actor together on a single HTTP port.
