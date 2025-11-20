# Metrics

The metrics actor provides observability for Ergo applications by collecting and exposing runtime statistics in Prometheus format. Instead of manually instrumenting your code with counters and gauges scattered throughout, the metrics actor centralizes telemetry into a single process that exposes an HTTP endpoint for Prometheus to scrape.

This approach separates monitoring concerns from application logic. Your actors focus on business functionality while the metrics actor handles collection, aggregation, and exposure of operational data. Prometheus or compatible monitoring systems poll the `/metrics` endpoint periodically, building time-series data for alerting and visualization.

## Why Monitor Actors

Actor systems present unique monitoring challenges. Traditional thread-based applications have predictable resource usage patterns - you monitor thread pools, request queues, and database connections. Actor systems are more dynamic - processes spawn and terminate constantly, messages flow asynchronously through mailboxes, and work distribution depends on supervision trees and message routing.

The metrics actor addresses this by tracking:

**Process metrics** - How many processes exist, how many are running vs. idle vs. zombie. This reveals whether your node is under load or experiencing process leaks.

**Memory metrics** - Heap allocation and actual memory used. Actor systems can accumulate small allocations across thousands of processes. Memory metrics help identify whether garbage collection keeps pace with allocation.

**Network metrics** - For distributed Ergo clusters, tracking bytes and messages flowing between nodes reveals network bottlenecks, routing inefficiencies, or failing connections.

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

2. **Base metrics collect automatically**. Node information (processes, memory, CPU) and network statistics (connected nodes, message rates) update at the configured interval.

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
- **CollectInterval**: `10 seconds`

The HTTP endpoint starts automatically during initialization. The first metrics collection happens immediately, and subsequent collections run at the configured interval.

## Configuration

Customize the HTTP endpoint and collection frequency:

```go
options := metrics.Options{
    Host:            "0.0.0.0",        // Listen on all interfaces
    Port:            9090,              // Prometheus default port
    CollectInterval: 5 * time.Second,  // Collect every 5 seconds
}

node.Spawn(metrics.Factory, gen.ProcessOptions{}, options)
```

**Host** determines which network interface the HTTP server binds to. Use `"localhost"` to restrict access to local connections only (development, testing). Use `"0.0.0.0"` to accept connections from any interface (production, containerized environments).

**Port** should not conflict with other services. Prometheus conventionally uses `9090`, but many Ergo applications use that for other purposes. Choose a port that doesn't collide with your application's HTTP servers, Observer UI (default `9911`), or other metrics exporters.

**CollectInterval** controls how frequently the actor queries node statistics. Shorter intervals provide more granular time-series data but increase CPU usage for collection. Longer intervals reduce overhead but miss short-lived spikes. For most applications, 10-15 seconds balances responsiveness with resource usage. Prometheus typically scrapes every 15-60 seconds, so collecting more frequently than your scrape interval wastes resources.

## Base Metrics

The metrics actor automatically exposes these Prometheus metrics without any configuration:

### Node Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `ergo_node_uptime_seconds` | Gauge | Time since node started. Useful for detecting node restarts and calculating availability. |
| `ergo_processes_total` | Gauge | Total number of processes including running, idle, and zombie. High counts suggest process leaks or inefficient cleanup. |
| `ergo_processes_running` | Gauge | Processes actively handling messages. Low relative to total suggests most processes are idle (good) or blocked (bad - investigate what they're waiting for). |
| `ergo_processes_zombie` | Gauge | Processes terminated but not yet fully cleaned up. These should be transient. Persistent zombies indicate bugs in termination handling. |
| `ergo_memory_used_bytes` | Gauge | Total memory obtained from OS (uses `runtime.MemStats.Sys`). |
| `ergo_memory_alloc_bytes` | Gauge | Bytes of allocated heap objects (uses `runtime.MemStats.Alloc`). |
| `ergo_cpu_user_seconds` | Gauge | CPU time spent executing user code. Increases as the node does work. Rate of change indicates CPU utilization. |
| `ergo_cpu_system_seconds` | Gauge | CPU time spent in kernel (system calls). High system time relative to user time suggests I/O bottlenecks or excessive syscalls. |
| `ergo_applications_total` | Gauge | Number of applications loaded. Should match your expected count. Unexpected changes indicate applications starting or stopping. |
| `ergo_applications_running` | Gauge | Applications currently active. Compare to total to identify stopped or failed applications. |
| `ergo_registered_names_total` | Gauge | Processes registered with atom names. High counts suggest heavy use of named processes for routing. |
| `ergo_registered_aliases_total` | Gauge | Total number of registered aliases. Includes aliases created by processes via `CreateAlias()` and aliases identifying meta-processes. |
| `ergo_registered_events_total` | Gauge | Event subscriptions active in the node. High counts indicate extensive pub/sub usage. |

### Network Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `ergo_connected_nodes_total` | Gauge | - | Number of remote nodes connected. For distributed systems, this should match your expected cluster size. |
| `ergo_remote_node_uptime_seconds` | Gauge | `node` | Uptime of each connected remote node. Resets when the remote node restarts. |
| `ergo_remote_messages_in_total` | Gauge | `node` | Messages received from each remote node. Rate indicates traffic volume. |
| `ergo_remote_messages_out_total` | Gauge | `node` | Messages sent to each remote node. Asymmetric in/out rates may reveal routing issues. |
| `ergo_remote_bytes_in_total` | Gauge | `node` | Bytes received from each remote node. Disproportionate bytes-to-messages ratio suggests large messages or inefficient serialization. |
| `ergo_remote_bytes_out_total` | Gauge | `node` | Bytes sent to each remote node. Monitors network bandwidth usage per peer. |

Network metrics use labels (`node="..."`) to separate per-node data. This creates multiple time series - one per connected node. Prometheus queries can aggregate across labels or filter to specific nodes.

## Custom Metrics

Extend the metrics actor by embedding `metrics.Actor`. You register custom Prometheus collectors in `Init()` and update them via `CollectMetrics()` or `HandleMessage()`.

### Approach 1: Periodic Collection

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

### Approach 2: Event-Driven Updates

Update metrics immediately when events occur:

```go
type AppMetrics struct {
    metrics.Actor

    requestsTotal  prometheus.Counter
    requestLatency prometheus.Histogram
}

func (m *AppMetrics) Init(args ...any) (metrics.Options, error) {
    m.requestsTotal = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "myapp_requests_total",
        Help: "Total requests processed",
    })

    m.requestLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
        Name:    "myapp_request_duration_seconds",
        Help:    "Request latency distribution",
        Buckets: prometheus.DefBuckets,
    })

    m.Registry().MustRegister(m.requestsTotal, m.requestLatency)

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

## Observer Integration

The metrics actor includes built-in Observer support via `HandleInspect()`. When you inspect it in Observer UI (http://localhost:9911), you see:

- Total number of registered metrics
- HTTP endpoint URL for Prometheus scraping
- Collection interval
- Current values for all metrics (base + custom)

This works automatically for custom metrics - register them in `Init()` and they appear in Observer alongside base metrics.

If you need custom inspection behavior, override `HandleInspect()` in your implementation:

```go
func (m *AppMetrics) HandleInspect(from gen.PID, item ...string) map[string]string {
    result := make(map[string]string)
    
    // Custom inspection logic
    result["status"] = "healthy"
    result["custom_info"] = "some value"
    
    return result
}
```

For detailed configuration options, see the `metrics.Options` struct and `ActorBehavior` interface in the package. For examples of custom metrics, see the [example directory](https://github.com/ergo-services/actor/tree/main/metrics/example).
