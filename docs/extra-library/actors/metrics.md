# Metrics

The metrics actor provides observability for Ergo applications by collecting and exposing runtime statistics in Prometheus format. Instead of manually instrumenting your code with counters and gauges scattered throughout, the metrics actor centralizes telemetry into a single process that exposes an HTTP endpoint for Prometheus to scrape.

This approach separates monitoring concerns from application logic. Your actors focus on business functionality while the metrics actor handles collection, aggregation, and exposure of operational data. Prometheus or compatible monitoring systems poll the `/metrics` endpoint periodically, building time-series data for alerting and visualization.

## Why Monitor Actors

Actor systems present unique monitoring challenges. Traditional thread-based applications have predictable resource usage patterns - you monitor thread pools, request queues, and database connections. Actor systems are more dynamic - processes spawn and terminate constantly, messages flow asynchronously through mailboxes, and work distribution depends on supervision trees and message routing.

The metrics actor addresses this by tracking:

**Process metrics** - How many processes exist, how many are running vs. idle vs. zombie. This reveals whether your node is under load or experiencing process leaks.

**Mailbox metrics** - Queue depth and latency for every process on the node. Depth shows how many messages are waiting in each mailbox; latency shows how long the oldest message has been waiting. Together they answer whether actors are keeping up with their workload and which specific processes are falling behind.

**Utilization and throughput metrics** - How much time each process spends executing callbacks relative to its lifetime, and how many messages flow through the node per second. These reveal compute-bound actors, idle capacity, and overall system throughput.

**Memory metrics** - Heap allocation and actual memory used. Actor systems can accumulate small allocations across thousands of processes. Memory metrics help identify whether garbage collection keeps pace with allocation.

**Network metrics** - For distributed Ergo clusters, tracking bytes and messages flowing between nodes reveals network bottlenecks, routing inefficiencies, or failing connections.

**Event metrics** - For pub/sub events, tracking which events have the most subscribers, which generate the most delivery load, and which are wasteful (publishing into the void or registered but unused). These reveal whether your event-driven architecture is efficient or accumulating overhead. See [Events](../../basics/events.md) for the pub/sub model and [Pub/Sub Internals](../../advanced/pub-sub-internals.md) for the shared subscription optimization that affects how delivery counters work.

**Application metrics** - How many applications are loaded and running. Applications failing to start or terminating unexpectedly appear in these counts.

These base metrics provide system-level visibility. For application-specific metrics (request rates, business transactions, custom counters), you extend the metrics actor with your own Prometheus collectors.

## ActorBehavior Interface

The metrics actor extends `gen.ProcessBehavior` with a specialized interface:

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

Only `Init()` is required - register your custom metrics and return options; all other callbacks have default implementations you can override as needed.

You have two main patterns:

**Periodic collection** - Implement `CollectMetrics()` to query state at intervals. Use when metrics reflect current state from other actors or external sources.

**Event-driven updates** - Implement `HandleMessage()` or `HandleEvent()` to update metrics when events occur. Use when your application produces natural event streams or publishes events.

## How It Works

When you spawn the metrics actor:

1. **HTTP endpoint starts** at the configured host and port. The `/metrics` endpoint immediately serves Prometheus-formatted data.

2. **Base metrics collect automatically**. Node information (processes, memory, CPU), network statistics (connected nodes, message rates), per-process metrics (mailbox depth, utilization, latency, aggregates), and per-event metrics (utilization state, subscriber counts, delivery rates) update at the configured interval.

3. **Custom metrics update** via `CollectMetrics()` callback or `HandleMessage()` processing, depending on your implementation.

4. **Prometheus scrapes** the `/metrics` endpoint and receives current values for all registered collectors (base + custom).

The actor handles HTTP serving and registry management. You focus on defining metrics and updating their values.

## Basic Usage

Spawn the metrics actor like any other process:

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

    // Spawn metrics actor with defaults
    node.Spawn(metrics.Factory, gen.ProcessOptions{}, metrics.Options{})

    // Metrics available at http://localhost:3000/metrics
    node.Wait()
}
```

Default configuration:
- **Host**: `localhost`
- **Port**: `3000`
- **Path**: `/metrics`
- **CollectInterval**: `10 seconds`
- **TopN**: `50`

The HTTP endpoint starts automatically during initialization. The first metrics collection happens immediately, and subsequent collections run at the configured interval.

## Configuration

Customize the HTTP endpoint and collection frequency:

```go
options := metrics.Options{
    Host:            "0.0.0.0",        // Listen on all interfaces
    Port:            9090,              // Prometheus default port
    Path:            "/metrics",       // HTTP path (default: "/metrics")
    CollectInterval: 5 * time.Second,  // Collect every 5 seconds
    TopN:            50,               // Top-N entries for each metric group (processes and events)
}

node.Spawn(metrics.Factory, gen.ProcessOptions{}, options)
```

**Host** determines which network interface the HTTP server binds to. Use `"localhost"` to restrict access to local connections only (development, testing). Use `"0.0.0.0"` to accept connections from any interface (production, containerized environments).

**Port** should not conflict with other services. Prometheus conventionally uses `9090`, but many Ergo applications use that for other purposes. Choose a port that doesn't collide with your application's HTTP servers, Observer UI (default `9911`), or other metrics exporters.

**Path** sets the HTTP path where the Prometheus handler is registered. Default is `"/metrics"`. Change it when the default path conflicts with your application's routing or when you need metrics at a non-standard location behind a reverse proxy.

**TopN** sets how many top entries are tracked for each metric group -- mailbox depth, utilization, latency for processes, and subscribers, published, local sent, remote sent for events (default: 50). Higher values provide more visibility but increase Prometheus cardinality. Set to 0 is not supported; the minimum effective value is 1.

**CollectInterval** controls how frequently the actor queries node statistics. Shorter intervals provide more granular time-series data but increase CPU usage for collection. Longer intervals reduce overhead but miss short-lived spikes. For most applications, 10-15 seconds balances responsiveness with resource usage. Prometheus typically scrapes every 15-60 seconds, so collecting more frequently than your scrape interval wastes resources.

**Mux** accepts an external `*http.ServeMux`. When provided, the metrics actor registers its handler on this mux and skips starting its own HTTP server. This is useful when you want to serve metrics alongside other HTTP handlers on a single port -- for example, combining the metrics endpoint with the [Health](health.md) actor endpoints or your own application handlers. When `Mux` is set, `Host` and `Port` are ignored.

```go
mux := http.NewServeMux()

// Metrics actor registers /metrics on this mux
metricsOpts := metrics.Options{
    Mux:             mux,
    CollectInterval: 5 * time.Second,
}
node.Spawn(metrics.Factory, gen.ProcessOptions{}, metricsOpts)

// Health actor registers /health/* on the same mux
healthOpts := health.Options{Mux: mux}
node.SpawnRegister("health", health.Factory, gen.ProcessOptions{}, healthOpts)

// Serve the shared mux yourself
```

## Base Metrics

The metrics actor automatically exposes these Prometheus metrics without any configuration:

### Node Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `ergo_node_uptime_seconds` | Gauge | Time since node started. Useful for detecting node restarts and calculating availability. |
| `ergo_processes_total` | Gauge | Total number of processes including running, idle, and zombie. High counts suggest process leaks or inefficient cleanup. |
| `ergo_processes_running` | Gauge | Processes actively handling messages. Low relative to total suggests most processes are idle (good) or blocked (bad - investigate what they're waiting for). |
| `ergo_processes_zombie` | Gauge | Processes terminated but not yet fully cleaned up. These should be transient. Persistent zombies indicate bugs in termination handling. |
| `ergo_processes_spawned_total` | Gauge | Cumulative number of successfully spawned processes since node start. Monotonically increasing counter useful for tracking spawn rate over time. |
| `ergo_processes_spawn_failed_total` | Gauge | Cumulative number of failed spawn attempts. Non-zero values indicate initialization errors or resource constraints preventing process creation. |
| `ergo_processes_terminated_total` | Gauge | Cumulative number of terminated processes. Compare to spawned count to understand process lifecycle patterns. |
| `ergo_memory_used_bytes` | Gauge | Total memory obtained from OS (uses `runtime.MemStats.Sys`). |
| `ergo_memory_alloc_bytes` | Gauge | Bytes of allocated heap objects (uses `runtime.MemStats.Alloc`). |
| `ergo_cpu_user_seconds` | Gauge | CPU time spent executing user code. Increases as the node does work. Rate of change indicates CPU utilization. |
| `ergo_cpu_system_seconds` | Gauge | CPU time spent in kernel (system calls). High system time relative to user time suggests I/O bottlenecks or excessive syscalls. |
| `ergo_cpu_cores` | Gauge | Number of CPU cores available to the process. Useful for normalizing CPU utilization metrics. |
| `ergo_applications_total` | Gauge | Number of applications loaded. Should match your expected count. Unexpected changes indicate applications starting or stopping. |
| `ergo_applications_running` | Gauge | Applications currently active. Compare to total to identify stopped or failed applications. |
| `ergo_registered_names_total` | Gauge | Processes registered with atom names. High counts suggest heavy use of named processes for routing. |
| `ergo_registered_aliases_total` | Gauge | Total number of registered aliases. Includes aliases created by processes via `CreateAlias()` and aliases identifying meta-processes. |
| `ergo_registered_events_total` | Gauge | Total number of registered events on this node. |
| `ergo_events_published_total` | Gauge | Cumulative number of events published by local producers. Use `rate()` to get publish throughput. |
| `ergo_events_received_total` | Gauge | Cumulative number of events received from remote nodes. Shows incoming event traffic load. |
| `ergo_events_local_sent_total` | Gauge | Cumulative number of event messages delivered to local subscribers. This reflects the actual fanout load -- a single publish with 100 subscribers produces 100 local deliveries. |
| `ergo_events_remote_sent_total` | Gauge | Cumulative number of event messages sent to remote nodes. Due to shared subscription optimization, one message is sent per remote node regardless of how many subscribers that node has. See [Pub/Sub Internals](../../advanced/pub-sub-internals.md). |

### Network Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `ergo_connected_nodes_total` | Gauge | - | Number of remote nodes connected. For distributed systems, this should match your expected cluster size. |
| `ergo_remote_node_uptime_seconds` | Gauge | `remote_node` | Uptime of each connected remote node. Resets when the remote node restarts. |
| `ergo_remote_messages_in_total` | Gauge | `remote_node` | Messages received from each remote node. Rate indicates traffic volume. |
| `ergo_remote_messages_out_total` | Gauge | `remote_node` | Messages sent to each remote node. Asymmetric in/out rates may reveal routing issues. |
| `ergo_remote_bytes_in_total` | Gauge | `remote_node` | Bytes received from each remote node. Disproportionate bytes-to-messages ratio suggests large messages or inefficient serialization. |
| `ergo_remote_bytes_out_total` | Gauge | `remote_node` | Bytes sent to each remote node. Monitors network bandwidth usage per peer. |

Network metrics use labels (`remote_node="..."`) to separate per-node data. This creates multiple time series - one per connected node. Prometheus queries can aggregate across labels or filter to specific nodes.

### Mailbox Latency Metrics

When built with `-tags=latency`, the metrics actor automatically collects per-process mailbox latency data. This enables detection of stressed processes whose mailboxes are growing.

```bash
go build -tags=latency ./...
```

Without the tag, latency measurement is disabled and no additional metrics are registered. There is zero overhead.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `ergo_mailbox_latency_distribution` | Gauge | `range` | Number of processes in each latency range. Snapshot per collect cycle -- values reflect the current state, not cumulative history. |
| `ergo_mailbox_latency_max_seconds` | Gauge | - | Maximum mailbox latency across all processes on this node. When this exceeds 1 second, at least one process is significantly behind. |
| `ergo_mailbox_latency_processes` | Gauge | - | Number of processes with non-empty mailbox (latency > 0). High count relative to total processes indicates widespread backpressure. |
| `ergo_mailbox_latency_top_seconds` | Gauge | `pid`, `name`, `application`, `behavior` | Top-N processes by mailbox latency. Directly identifies which processes are the bottlenecks. |

The distribution metric uses gauge-based snapshots rather than a Prometheus histogram. Each collect cycle iterates over all processes, counts how many fall into each latency range, and sets the gauge values from scratch. This approach is a better fit for periodic state observation than cumulative histograms, which are designed for discrete events like HTTP requests. The ranges are: 1ms, 5ms, 10ms, 50ms, 100ms, 500ms, 1s, 5s, 10s, 30s, 60s, and 60s+. Each range represents an upper boundary -- for example, "5ms" counts processes with latency between 1ms and 5ms.

The `TopN` option (default: 50) controls how many processes appear in the top-N metric. The same setting applies to all per-process top-N metrics (latency, depth, utilization).

### Mailbox Depth Metrics

The metrics actor collects per-process mailbox queue depth -- the number of messages waiting in the mailbox at the moment of collection. While latency measures how long the oldest message has been waiting, depth measures how many messages are queued. The two metrics are complementary: a process may have high depth with low latency if it processes messages quickly but receives many at once, or low depth with high latency if a single message is taking a long time to process.

No build tags required. Depth metrics are always active.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `ergo_mailbox_depth_distribution` | Gauge | `range` | Number of processes in each depth range. Snapshot per collect cycle. |
| `ergo_mailbox_depth_max` | Gauge | - | Maximum mailbox depth across all processes on this node. |
| `ergo_mailbox_depth_top` | Gauge | `pid`, `name`, `application`, `behavior` | Top-N processes by mailbox depth. |

Distribution ranges: 1, 5, 10, 50, 100, 500, 1K, 5K, 10K, 10K+. Each range represents an upper boundary. Processes with empty mailboxes are not counted.

### Process Utilization Metrics

The metrics actor collects per-process utilization -- the ratio of callback running time to process uptime. A process that has been alive for 100 seconds and spent 30 of those seconds inside callbacks has a utilization of 0.30 (30%). This is a lifetime average computed from cumulative counters that the framework maintains for each process. It answers the question "which actors have been busiest over their entire lifetime?"

Utilization is not the same as current CPU load. A process that was heavily loaded an hour ago but is idle now will still show high lifetime utilization. For current load, the dashboard provides `rate(ergo_process_running_time_seconds)` which shows how much callback time is happening right now per second.

No build tags required. Utilization metrics are always active.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `ergo_process_utilization_distribution` | Gauge | `range` | Number of processes in each utilization range. Snapshot per collect cycle. |
| `ergo_process_utilization_max` | Gauge | - | Maximum process utilization on this node. |
| `ergo_process_utilization_top` | Gauge | `pid`, `name`, `application`, `behavior` | Top-N processes by utilization. |

Distribution ranges: 1%, 5%, 10%, 25%, 50%, 75%, 90%, 90%+. Processes with zero running time or zero uptime are excluded. Utilization is capped at 1.0 (100%).

### Process Init Time Metrics

The metrics actor tracks how long each process spent in its `ProcessInit` callback. This identifies actors with slow initialization -- heavy setup, blocking I/O, or synchronous calls during init. The default init timeout is 5 seconds (`DefaultRequestTimeout`), maximum 15 seconds for remote spawn.

No build tags required. Init time metrics are always active.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `ergo_process_init_time_max_seconds` | Gauge | - | Maximum ProcessInit duration across all processes on this node. |
| `ergo_process_init_time_top_seconds` | Gauge | `pid`, `name`, `application`, `behavior` | Top-N processes by ProcessInit duration. |

### Process Throughput Metrics

The metrics actor tracks per-process message throughput (top-N by messages received and sent) and computes node-level aggregate counters by summing per-process values.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `ergo_process_messages_in_top` | Gauge | `pid`, `name`, `application`, `behavior` | Top-N processes by total messages received. Identifies which actors handle the most inbound traffic. |
| `ergo_process_messages_out_top` | Gauge | `pid`, `name`, `application`, `behavior` | Top-N processes by total messages sent. Identifies which actors generate the most outbound traffic. |
| `ergo_process_messages_in` | Gauge | - | Sum of messages received by all processes on this node. |
| `ergo_process_messages_out` | Gauge | - | Sum of messages sent by all processes on this node. |
| `ergo_process_running_time_seconds` | Gauge | - | Sum of callback running time across all processes on this node (seconds). |

Aggregate values are cumulative -- apply `rate()` in Prometheus to get per-second rates. When a process terminates, its contribution is removed from the sum, which may cause the aggregate to decrease momentarily. This is expected and `rate()` handles it correctly in most cases.

`rate(ergo_process_messages_in)` and `rate(ergo_process_messages_out)` give the node-level message throughput in messages per second. `rate(ergo_process_running_time_seconds)` gives the node-level actor CPU utilization in seconds of callback execution per second -- when this value approaches the number of available CPU cores, the node is compute-saturated.

### Process Wakeups and Drains Metrics

The metrics actor tracks process wakeups (transitions from Sleep to Running state) and drains (messages processed per wakeup). A wakeup occurs each time the framework starts a new goroutine to handle messages in a process's mailbox. The drain ratio (`MessagesIn / Wakeups`) reveals the nature of a process's load that utilization alone cannot distinguish. Two processes with 80% utilization may have completely different workloads: one with drain ~1 processes individual messages slowly (heavy per-message computation), while one with drain ~100 processes messages quickly but receives so many that it never sleeps (high throughput load). The optimization strategy is different: the first needs faster callbacks, the second needs load distribution.

No build tags required. Wakeups and drains metrics are always active.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `ergo_process_wakeups_top` | Gauge | `pid`, `name`, `application`, `behavior` | Top-N processes by cumulative wakeup count. |
| `ergo_process_drains_top` | Gauge | `pid`, `name`, `application`, `behavior` | Top-N processes by drain ratio (MessagesIn / Wakeups). |
| `ergo_process_wakeups` | Gauge | - | Sum of wakeups across all processes on this node. |

On the dashboard, the Throughput panels show wakeup rate as a third line alongside message in/out rates -- the visual gap between message rate and wakeup rate represents the drain effect. Drains per Node timeseries shows per-node drain ratio over time computed from aggregate metrics: `rate(messages_in) / rate(wakeups)`.

### Event Metrics

The metrics actor collects per-event pub/sub metrics using `Node.EventRangeInfo()`, which iterates over all registered events and returns their current statistics. This provides visibility into the pub/sub layer: which events have the most subscribers, which generate the most delivery load, and which are wasteful. The subscriber count for each event includes both `LinkEvent` and `MonitorEvent` subscribers.

No build tags required. Event metrics are always active.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `ergo_event_subscribers_max` | Gauge | - | Maximum subscriber count across all events on this node. |
| `ergo_event_utilization` | Gauge | `state` | Number of events in each utilization state. Snapshot per collect cycle. |
| `ergo_event_subscribers_top` | Gauge | `event`, `producer` | Top-N events by subscriber count. |
| `ergo_event_published_top` | Gauge | `event`, `producer` | Top-N events by messages published. |
| `ergo_event_local_sent_top` | Gauge | `event`, `producer` | Top-N events by messages delivered to local subscribers. |
| `ergo_event_remote_sent_top` | Gauge | `event`, `producer` | Top-N events by messages sent to remote nodes. |

The utilization metric classifies every registered event into exactly one state:

- **`active`** -- has both published messages and subscribers. The event is doing its job -- producing and delivering messages.
- **`on_demand`** -- the event was registered with `Notify` enabled and is currently waiting (either no subscribers yet, or subscribers present but producer hasn't started). This is the expected state for on-demand producers that use `MessageEventStart`/`MessageEventStop` to control when they publish. See [Events](../../basics/events.md) for the `Notify` mechanism.
- **`idle`** -- registered without `Notify`, has zero subscribers and zero publishes. Likely a forgotten event consuming resources without purpose.
- **`no_subscribers`** -- has published messages but currently no subscribers, registered without `Notify`. The producer is doing work (serialization, message construction) for nothing.
- **`no_publishing`** -- has subscribers waiting but the producer has never published, registered without `Notify`. Could indicate a bug where the producer publishes to the wrong event name, or a producer that hasn't received its data source yet.

The total across all states equals the total number of registered events on the node. A healthy system shows mostly `active` and `on_demand`. Growth in `idle` or `no_subscribers` over time indicates accumulating waste.

The distinction between `published`, `local_sent`, and `remote_sent` in the top-N metrics reflects the pub/sub delivery model. A single publish fans out to all local subscribers (so `local_sent` can be much larger than `published`) and sends one message per remote subscriber node (due to the [shared subscription optimization](../../advanced/pub-sub-internals.md)). Comparing these values reveals the actual delivery cost of each event.

### Per-Process Metrics Collection

All per-process metrics (latency, depth, utilization, throughput, wakeups/drains, init time, aggregates) are collected in a single pass using `Node.ProcessRangeShortInfo()`. The iterator visits each process once, and each observation is dispatched to the latency, depth, utilization, and throughput collectors simultaneously. Event metrics are collected separately using `Node.EventRangeInfo()`, which iterates over all registered events. Both iterators use snapshot-then-iterate: the data is captured under a read lock, then the lock is released before the callback runs, so collection does not block producers or subscribers. Top-N selection uses a min-heap for O(N) efficiency.

## Custom Metrics

There are three approaches to custom metrics depending on your use case: helper functions from any actor, embedding `metrics.Actor` for direct registry access, and shared mode for high-throughput scenarios.

### Helper Functions

Any actor on the same node can register and update custom metrics without importing `prometheus` or embedding the metrics actor. Registration is a synchronous Call (returns error on failure). Updates are asynchronous Send (fire-and-forget).

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

The first argument is the calling process (`gen.Process`), the second is the target metrics actor (name, PID, or alias). When the registering process terminates, the metrics actor automatically unregisters all metrics it owned.

This is the simplest approach. Use it when actors just need to report values without owning Prometheus collector objects.

### Embedding metrics.Actor

For direct access to the Prometheus registry, periodic collection via `CollectMetrics`, or event-driven updates via `HandleMessage`, embed `metrics.Actor`.

#### Periodic Collection

Implement `CollectMetrics()` to poll state at regular intervals:

```go
type AppMetrics struct {
    metrics.Actor

    activeUsers   prometheus.Gauge
    queueDepth    prometheus.Gauge
}

func (m *AppMetrics) Init(args ...any) (metrics.Options, error) {
    m.activeUsers = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "myapp_active_users",
        Help: "Current number of active users",
    })

    m.queueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "myapp_queue_depth",
        Help: "Current queue depth",
    })

    m.Registry().MustRegister(m.activeUsers, m.queueDepth)

    return metrics.Options{
        Port:            9090,
        CollectInterval: 5 * time.Second,
    }, nil
}

func (m *AppMetrics) CollectMetrics() error {
    // Called every CollectInterval
    // Query other processes for current state
    
    count, err := m.Call(userService, getActiveUsersMessage{})
    if err != nil {
        m.Log().Warning("failed to get user count: %s", err)
        return nil // Non-fatal, continue
    }
    m.activeUsers.Set(float64(count.(int)))
    
    depth, _ := m.Call(queueService, getDepthMessage{})
    m.queueDepth.Set(float64(depth.(int)))
    
    return nil
}
```

Use this when metrics reflect state you need to query - current values from other actors, computed aggregates, external API calls.

#### Event-Driven Updates

Update metrics immediately when events occur:

```go
type AppMetrics struct {
    metrics.Actor

    requestsTotal  prometheus.Counter
    errorsTotal    prometheus.Counter
    requestLatency prometheus.Histogram
}

func (m *AppMetrics) Init(args ...any) (metrics.Options, error) {
    m.requestsTotal = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "myapp_requests_total",
        Help: "Total requests processed",
    })

    m.errorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "myapp_errors_total",
        Help: "Total errors occurred",
    })

    m.requestLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
        Name:    "myapp_request_duration_seconds",
        Help:    "Request latency distribution",
        Buckets: prometheus.DefBuckets,
    })

    m.Registry().MustRegister(m.requestsTotal, m.errorsTotal, m.requestLatency)

    return metrics.Options{Port: 9090}, nil
}

func (m *AppMetrics) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case requestCompletedMessage:
        m.requestsTotal.Inc()
        m.requestLatency.Observe(msg.duration.Seconds())
    case errorOccurredMessage:
        m.errorsTotal.Inc()
    }
    return nil
}
```

Application actors send events to the metrics actor:

```go
// In your request handler actor
func (h *RequestHandler) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case ProcessRequest:
        start := time.Now()
        // ... process request ...
        elapsed := time.Since(start)
        
        // Send metrics event
        h.Send(metricsPID, requestCompletedMessage{duration: elapsed})
    }
    return nil
}
```

Use this when your application naturally produces events. Metrics update in real-time without polling.

### Shared Mode

A single metrics actor handles all custom metric operations sequentially -- registration, updates, and unregistration pass through one mailbox. Under high throughput this becomes a bottleneck: hundreds of actors sending gauge updates or histogram observations compete for the same process, and the mailbox grows faster than callbacks can drain it.

Shared mode solves this by separating the Prometheus registry from the actor. You create a `metrics.Shared` object that holds the registry and a thread-safe map of registered metrics. Multiple metrics actor instances share this object -- each one can serve updates independently because Prometheus collectors are safe for concurrent use. Registration still serializes through a single actor (to avoid duplicate metric names), but updates go to any worker in a pool.

Create the shared object and pass it to all metrics actors that should share the same registry:

```go
shared := metrics.NewShared()

// Primary actor: owns the HTTP endpoint and base Ergo metrics
primaryOpts := metrics.Options{
    Port:   9090,
    Shared: shared,
}

// Worker actors: handle custom metric updates only (no HTTP, no base metrics)
workerOpts := metrics.Options{
    Shared: shared,
}
```

The primary actor starts the HTTP server and collects base Ergo metrics as usual. Worker actors skip HTTP and base collection -- they only process custom metric messages (register, update, unregister). All actors write to the same Prometheus registry through the shared object.

This pattern works well with `act.Pool`: put worker actors behind a pool, and the pool distributes incoming metric updates across workers automatically. The `application/radar` package uses this approach internally.

## Metric Types

Prometheus defines four metric types, each suited for different use cases:

**Counter** - Monotonically increasing value. Use for events that accumulate (requests processed, errors occurred, bytes sent). Counters never decrease except on process restart. Prometheus queries typically use `rate()` to calculate per-second rates or `increase()` for total change over a time window.

**Gauge** - Value that can go up or down. Use for current state (active connections, queue depth, memory usage, CPU utilization). Gauges represent snapshots. Prometheus queries can graph them directly or use functions like `avg_over_time()` to smooth spikes.

**Histogram** - Observations bucketed into configurable ranges. Use for latency or size distributions. Histograms let you calculate percentiles (p50, p95, p99) in Prometheus queries. They're more resource-intensive than gauges because they maintain multiple buckets per metric.

**Summary** - Similar to histogram but calculates quantiles client-side. Use when you need precise quantiles but can't predict bucket boundaries. Summaries are more expensive than histograms because they track exact quantiles, not approximations.

For most use cases, counters and gauges suffice. Use histograms when you need latency percentiles. Avoid summaries unless you have specific reasons - histograms are more flexible for Prometheus queries.


## Integration with Prometheus

Configure Prometheus to scrape the metrics endpoint:

```yaml
scrape_configs:
  - job_name: 'ergo-nodes'
    static_configs:
      - targets:
          - 'localhost:3000'
          - 'node1.example.com:3000'
          - 'node2.example.com:3000'
    scrape_interval: 15s
```

Prometheus fetches `/metrics` every 15 seconds, parses the text format, and stores time-series data. You can then query, alert, and visualize metrics using Prometheus queries or Grafana dashboards.

For dynamic discovery in Kubernetes or cloud environments, use Prometheus service discovery instead of static targets. The metrics actor itself doesn't need to know about Prometheus - it just exposes an HTTP endpoint.

## Grafana Dashboard

The metrics package includes a pre-built Grafana dashboard (`ergo-cluster.json`) designed for monitoring Ergo clusters. The dashboard provides a comprehensive view of cluster health with automatic refresh every 10 seconds.

### Importing the Dashboard

1. Open Grafana and navigate to Dashboards
2. Click "Import"
3. Upload the `ergo-cluster.json` file from the metrics package or paste its contents
4. Select your Prometheus data source

The dashboard includes a `$node` variable dropdown that filters all panels by selected nodes. By default, all nodes are displayed.

### Understanding the Panels

The dashboard organizes metrics into logical groups arranged from high-level overview at the top to detailed breakdowns below. Rows marked "collapsed" are hidden by default -- click the row header to expand them.

**Summary Row** (expanded) - Six stat panels showing aggregated values: total processes, running processes, zombie count (red when non-zero), memory used, memory allocated, and node count. These provide immediate cluster health at a glance. The gap between total and running is normal -- most processes spend their time in Sleep state (idle, waiting for messages). Running counts only processes currently executing callbacks or waiting for a Call response. Non-zero zombies require investigation.

**Mailbox Latency** (expanded, requires `-tags=latency`) - Six panels for latency analysis described in detail in the next section. When the `latency` tag is not used, these panels show "No data".

**Mailbox Depth** (expanded) - Three panels showing mailbox queue depth. Max Depth per Node tracks the largest mailbox on each node over time. Depth Distribution is a stacked area chart with a flame color gradient (green for 1-10 messages, yellow for 50-100, orange for 500-1K, red for 5K-10K+) showing how many processes fall into each depth range. Top Processes by Depth is a table listing the processes with the deepest queues across the cluster. Depth is complementary to latency: depth tells you "how many messages are queued," while latency tells you "how long the oldest one has been waiting."

**Events** (collapsed) - Ten panels for pub/sub event observability, organized from general to specific. Event Publish/Delivery Rate shows cluster-wide throughput with four lines: Published (local producer publish rate), Received (events arriving from remote nodes), Local Delivered (actual fanout to local subscribers), and Remote Sent (one message per remote node due to [shared subscriptions](../../advanced/pub-sub-internals.md)). Event Utilization is a stacked timeseries showing the state of all registered events: active (publishing with subscribers), on demand (using Notify, waiting), idle, no subscribers, no publishing. The total height equals total registered events; a healthy system is mostly green (active) and blue (on demand). Event Publish Rate per Node and Event Delivery Rate per Node show per-node publish and delivery rates. Registered Events per Node and Max Subscribers per Node provide per-node counts. Four tables at the bottom show Top Events by Subscribers, Published, Local Deliveries, and Remote Sent -- identifying which specific events create the most load. See [Events](../../basics/events.md) for the pub/sub model.

**Process Activity** (collapsed) - Ten panels organized by topic: message throughput, drains, then process utilization. Message Throughput (Cluster Total) and Message Throughput per Node show inbound, outbound, and wakeup rates -- the gap between message rate and wakeup rate reveals drain effect (processes batching under load). Top Processes by Messages In/Out tables identify which actors handle the most traffic. Drains per Node timeseries shows per-node drain ratio over time (`rate(messages_in) / rate(wakeups)`), and Top Processes by Drains table identifies specific actors with the highest drain -- these are the processes that handle the most messages per wakeup cycle. Drain ratio complements utilization: two processes at 80% utilization may have drain ~1 (slow callbacks) or drain ~100 (fast callbacks, high volume) -- different problems needing different solutions. Utilization Distribution, Max Utilization per Node, Actor Running Time per Node, and Top Processes by Utilization cover compute load analysis.

**Processes** (collapsed) - Six panels showing per-node process counts (total and running), lifecycle rates (spawn rate with failures in red, termination rate), and initialization performance (init time bar gauge per node, top processes by init time). Steady growth in total without plateau suggests process leaks. Spawn failures indicate resource exhaustion. When termination rate exceeds spawn rate, the node is draining. High init times indicate heavy initialization logic or blocking operations in ProcessInit.

**Resources** (collapsed) - Four panels covering CPU and memory. CPU User Time and CPU System Time are normalized by core count and displayed as percentages. High user CPU means compute-bound workload; high system CPU relative to user suggests excessive I/O or syscalls. Memory (OS:used) and Memory (Runtime:alloc) show memory usage over time. Monotonic growth signals memory leaks. Sawtooth pattern in runtime allocation is normal (GC cycles). Rising baseline between GC cycles indicates uncollected objects.

**Network** (collapsed) - Six panels covering cluster totals, per-node breakdowns, and node-pair detail for both message rates and byte rates. Sudden drops may indicate partitions. Disproportionate bytes-to-messages ratio reveals large message sizes. The detail panels show traffic between specific node pairs, useful for tracing inter-node communication paths and identifying saturated links.

**Nodes Overview** - A table listing all nodes with uptime, process counts, and memory. Sorted by process count. Quickly identifies recently restarted nodes (low uptime), overloaded nodes (high process count), or unhealthy nodes (non-zero zombies).

### Working with the Dashboard

The dashboard is designed around a top-down investigation pattern. Start with the Summary row for cluster health, then drill into the relevant section based on the symptom you observe.

#### Routine check

Open the dashboard and scan the Summary row:

- **Zombie count should be zero.** Non-zero means processes have terminated abnormally and were not cleaned up.
- **Node count should match your expectation.** A drop means a node left the cluster.
- **Memory used should be within expected bounds.** A sharp increase suggests a leak or load spike.

Then check the expanded rows below the Summary:

- **Mailbox Latency** (if using `-tags=latency`) -- Max Latency under 100ms and Stressed Processes mostly empty or light-blue means healthy. If not using latency, check Mailbox Depth instead -- zero or low depth across all nodes is healthy.
- **Mailbox Depth** -- Max Depth per Node at zero or near zero is normal.

For a deeper check, expand the collapsed rows:

- **Events** -- Event Utilization should be mostly green (active) and blue (on demand). Growth in grey (idle) or orange (no subscribers) over time indicates accumulating waste.
- **Process Activity** -- Message Throughput should be stable. A sudden drop compared to the previous period may indicate stalled processes.

#### Investigating backpressure

Backpressure means processes are receiving messages faster than they can handle them. The Mailbox Latency and Mailbox Depth rows are the primary tools.

**With `-tags=latency` enabled:**

Max Latency shows the worst case across the cluster. Stressed Processes shows how many processes are affected. Read them together:

- Max Latency spikes above 1 second -- at least one process is severely behind. Use the Top Stressed Processes table to identify it.
- Orange area growing in Stressed Processes -- multiple processes are falling behind. Check Latency Distribution to assess severity (isolated red sliver = one stuck process, chart shifting to orange = widespread degradation).
- Max Latency persistently elevated (minutes, not seconds) -- a stuck process, not a burst. Find it in the Top Stressed Processes table.

Use Max Latency per Node and Stressed Processes per Node to narrow down: one node standing out = localized problem; all nodes affected = systemic issue (shared dependency, deployment, cluster-wide traffic pattern).

**Without latency tag:**

Max Depth per Node is the closest alternative. Rising depth means a process accumulates messages faster than it handles them. Depth Distribution shows severity. Top Processes by Depth identifies the specific actors. Depth is complementary to latency: depth tells you "how many messages are queued," latency tells you "how long the oldest has been waiting."

#### Investigating throughput anomalies

Expand the Process Activity row. The top two rows cover message throughput.

- **Cluster Throughput drops suddenly** -- processes may be stalled or an upstream source stopped sending. Check if specific nodes dropped (Throughput per Node) or the entire cluster.
- **Cluster Throughput spikes** -- correlate with Mailbox Depth and Latency. A spike followed by growing depth means the system cannot absorb the burst.
- **In/Out imbalance** -- in a fully monitored cluster (metrics actor on every node), In and Out should be approximately balanced. A small imbalance is normal due to framework-internal messages (exit signals, down notifications) that increment In without a corresponding Out. Persistent significant imbalance indicates nodes without metrics collection: Out > In means messages are sent to unmonitored nodes, In > Out means unmonitored nodes are sending messages into the cluster.
- **One node has disproportionate throughput** -- Throughput per Node identifies hotspots. Cross-reference with Top Processes by Messages In/Out to find the specific actors.
- **Wakeup rate diverges from message rate** -- on Throughput panels, the gap between In and Wakeups lines shows drain effect. Growing gap means processes batch more messages per wakeup (increasing load). Drains per Node timeseries shows which node, Top Processes by Drains shows which process.

#### Investigating process lifecycle issues

Expand the Processes row.

- **Steady growth in total process count** without plateau -- process leak. Processes are spawned but never terminated. Check which application spawns them using the process tables.
- **Spawn failures** (red in Process Spawn Rate) -- resource exhaustion or configuration errors preventing process creation.
- **Spawn rate spikes** -- may signal a supervisor restart loop. Correlate with termination rate -- if both spike together, a process keeps crashing and restarting.
- **Termination rate exceeds spawn rate** -- the node is draining. Check why processes are terminating (errors, shutdown, kills).
- **High init time** -- Init Time per Node bar gauge shows the slowest ProcessInit on each node with color-coded severity. Default timeout is 5 seconds. Top Processes by Init Time table identifies which actor types take the longest. High init times indicate heavy setup, blocking I/O, or synchronous calls during initialization. If init times approach the timeout, processes risk being killed before they finish starting.

#### Investigating event issues

Expand the Events row.

- **Event Utilization shifting from green/blue to grey/orange** -- accumulating waste. Grey (idle) events are registered but unused. Orange (no subscribers) events are publishing into the void. Investigate which events are affected using the top-N tables.
- **Publish/Delivery Rate gap growing** -- Local Delivered >> Published indicates high fanout. If this correlates with latency increases on subscriber nodes, event fanout is causing backpressure.
- **One node has high Event Delivery Rate** -- that node handles the most event fanout. Cross-reference with Mailbox Depth and Latency for the same node.
- **Registered Events growing without plateau** -- event registration leak. Events are registered but never unregistered. Check Event Utilization for growing idle count.
- **Max Subscribers spike** -- one event suddenly gained many subscribers, amplifying the cost of each publish.

#### Investigating resource saturation

Expand the Resources row.

- **CPU User Time approaching 100%** -- compute-bound. Correlate with Utilization Distribution in Process Activity to see which processes are consuming CPU. Actor Running Time per Node approaching CPU core count confirms saturation.
- **High System CPU relative to User CPU** -- excessive syscalls, context switching, or I/O pressure rather than application workload.
- **Memory monotonically growing** -- memory leak. Compare OS:used across nodes to spot outliers. Check if Mailbox Depth is also growing -- accumulating messages consume memory.
- **Runtime:alloc baseline rising between GC cycles** -- objects not being collected. The sawtooth pattern should return to a stable baseline.

#### Investigating network issues

Expand the Network row.

- **Cluster total message rate drops suddenly** -- possible network partition or node failure. Check Node count in Summary and per-node breakdown.
- **One node-pair has disproportionate traffic** -- Network Detail panels show traffic between specific pairs. Identifies saturated links or unexpected communication patterns.
- **Bytes-to-messages ratio changing** -- average message size is growing. May indicate serialization issues or bulk data transfers.

#### Cross-panel correlation

Combining signals from different sections identifies root causes:

- **High latency + high depth** -- process is both slow and receiving more than it can handle. Mailbox growing. Clearest sign of overload.
- **High latency + low depth** -- single message processed slowly (long callback) or process blocked on external resource. Mailbox not growing.
- **High depth + low latency** -- process receives bursts but handles them quickly. Usually transient.
- **High latency + high CPU** -- compute-bound. Actors doing heavy work in callbacks. Distribute work or offload computation.
- **High latency + low CPU** -- processes blocked on something other than computation. External I/O, synchronous calls to other actors, or contention.
- **High latency + growing memory** -- mailboxes accumulating messages. If this continues, the node runs out of memory.
- **High latency + network traffic spike** -- burst of remote messages overwhelming receivers. Check Network Detail panels.
- **High event delivery rate + high latency on same node** -- event fanout causing backpressure. Top Events by Local Deliveries identifies which event, Mailbox Depth/Latency shows the impact on subscribers.
- **High init time + spawn rate spikes** -- supervisor restart loop with slow init. Each restart takes seconds, creating cascading delays.
- **Throughput drop + stable process count** -- processes alive but not doing work. Blocked on external calls or upstream failure (no messages arriving).
- **Running time per node approaching CPU cores** -- node is compute-saturated. All CPU time spent in actor callbacks. Reduce workload or add capacity.
- **Event no_subscribers growing + stable throughput** -- producers publishing into the void. Not using [Notify](../../basics/events.md) mechanism to detect absent subscribers.
- **High drain + stable depth** -- process running at maximum capacity but keeping up. No action needed unless depth starts growing.
- **High drain + growing depth** -- process cannot keep up despite continuous processing. Needs load distribution or faster message handling.
- **High utilization + low drain** -- slow per-message processing. Each callback takes long. Optimize the callback logic.
- **High utilization + high drain** -- fast processing but overwhelming volume. Distribute load across more processes.

## Observer Integration

The metrics actor includes built-in Observer support via `HandleInspect()`. When you inspect it in Observer UI (http://localhost:9911), you see:

- Total number of registered metrics
- HTTP endpoint URL for Prometheus scraping
- Collection interval
- Current values for all metrics (base + custom)

This works automatically for custom metrics -- register them in `Init()` and they appear in Observer alongside base metrics.

If you embed `metrics.Actor` and override `HandleInspect()`, the base inspection data (endpoint, interval, metric values) is always included. Your returned keys are merged on top, so you can add custom fields or override base values:

```go
func (m *AppMetrics) HandleInspect(from gen.PID, item ...string) map[string]string {
    result := make(map[string]string)
    // Add custom fields -- these merge with base data
    result["status"] = "healthy"
    result["custom_info"] = "some value"
    return result
}
```

For detailed configuration options, see the `metrics.Options` struct and `ActorBehavior` interface in the package. For examples of custom metrics, see the [metrics actor repository](https://github.com/ergo-services/actor).

## Radar Application

If your node needs both Prometheus metrics and Kubernetes health probes, consider the [Radar](../applications/radar.md) application. It runs the metrics actor and health actor together on a single HTTP port and provides helper functions so your actors don't need to import either package directly.
