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
    LatencyTopN:     50,               // Top-N processes by mailbox latency
}

node.Spawn(metrics.Factory, gen.ProcessOptions{}, options)
```

**Host** determines which network interface the HTTP server binds to. Use `"localhost"` to restrict access to local connections only (development, testing). Use `"0.0.0.0"` to accept connections from any interface (production, containerized environments).

**Port** should not conflict with other services. Prometheus conventionally uses `9090`, but many Ergo applications use that for other purposes. Choose a port that doesn't collide with your application's HTTP servers, Observer UI (default `9911`), or other metrics exporters.

**LatencyTopN** sets how many top processes by mailbox latency are tracked (default: 50). Only effective when built with `-tags=latency`. Higher values provide more visibility but increase Prometheus cardinality.

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

The `LatencyTopN` option (default: 50) controls how many processes appear in the top-N metric. For clusters with many nodes, consider the cardinality impact: each node contributes up to `LatencyTopN` time series for this metric.

**Cardinality estimate** for a cluster of 500 nodes with `LatencyTopN=50`:
- Distribution: 500 x 12 series = 6,000
- Max + Count gauges: 500 x 2 = 1,000
- Top-N gauges: 500 x 50 = 25,000
- Total: ~32,000 series

The collection uses `Node.ProcessRangeShortInfo()` to iterate over all processes efficiently in a single pass, computing the distribution, max, stressed count, and top-N simultaneously using a min-heap for O(N) selection.

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

## Grafana Dashboard

The metrics package includes a pre-built Grafana dashboard (`ergo-cluster.json`) designed for monitoring Ergo clusters. The dashboard provides a comprehensive view of cluster health with automatic refresh every 10 seconds.

### Importing the Dashboard

1. Open Grafana and navigate to Dashboards
2. Click "Import"
3. Upload the `ergo-cluster.json` file from the metrics package or paste its contents
4. Select your Prometheus data source

The dashboard includes a `$node` variable dropdown that filters all panels by selected nodes. By default, all nodes are displayed.

### Understanding the Panels

The dashboard organizes metrics into logical groups that answer operational questions:

**Summary Row** - Six stat panels showing aggregated values: total processes, running processes, zombie count (red when non-zero), memory used, memory allocated, and node count. These provide immediate cluster health at a glance. A gap between total and running processes indicates idle capacity or blocked processes. Non-zero zombies require investigation.

**Mailbox Latency** (expanded row, requires `-tags=latency`) - Appears right after the Summary row. Contains six panels for latency analysis described in detail in the next section. When the `latency` tag is not used, these panels show "No data".

**Processes** (collapsed row) - Four timeseries panels showing per-node process counts (total and running) and lifecycle rates (spawn rate with failures in red, termination rate). Click the row header to expand. Steady growth in total without plateau suggests process leaks. Compare running counts across nodes to identify load imbalance. Spawn failures indicate resource exhaustion. When termination rate exceeds spawn rate, the node is draining.

**CPU** - User and system CPU time normalized by core count, displayed as percentages. High user CPU means compute-bound workload. High system CPU relative to user suggests excessive I/O or syscalls rather than application work.

**Memory** - OS-reported memory and Go runtime allocation over time. Monotonic growth signals memory leaks. Sawtooth pattern in runtime allocation is normal (GC cycles). Rising baseline between GC cycles indicates uncollected objects.

**Network** - Four panels covering cluster totals and per-node breakdowns for message rates and byte rates. Sudden drops may indicate partitions. Disproportionate bytes-to-messages ratio reveals large message sizes.

**Network Detail** - Message and byte rates between specific node pairs. Useful for tracing inter-node communication paths and identifying saturated links.

**Nodes Overview** - A table listing all nodes with uptime, process counts, and memory. Sorted by process count. Quickly identifies recently restarted nodes (low uptime), overloaded nodes (high process count), or unhealthy nodes (non-zero zombies).

### Reading the Latency Dashboard

The latency row is the primary tool for answering "are my actors keeping up with their workload?" It contains six panels organized in three tiers: cluster overview at the top, per-node breakdown in the middle, and detailed drill-down at the bottom.

#### Start with the top row

The **Max Latency** panel (left) and **Stressed Processes** panel (right) sit at the top of the latency section. Together they answer the most basic operational question: is there a problem right now?

Max Latency shows the highest mailbox latency across all selected nodes as a red timeseries. The value represents how long the oldest unprocessed message has been sitting in some process's mailbox. Under normal conditions this stays under 100ms. Values above 1 second mean at least one process is significantly behind -- it is either overloaded, stuck in a long-running callback, or waiting for an external resource.

Stressed Processes shows a stacked area chart with two layers. The light-blue area represents processes with latency under 1ms -- these are technically non-empty mailboxes but the delay is negligible and considered normal operation. The orange area represents processes with latency of 1ms or above -- these are the ones worth investigating. In a healthy system the orange area should be absent or thin. A growing orange area indicates that more and more processes are falling behind.

If both panels look calm -- Max Latency under 100ms, no orange in Stressed Processes -- the system is healthy. No further investigation needed.

#### React to these signals

**Max Latency spikes above 1 second.** At least one process is severely behind. This is the strongest signal that something is wrong. Scroll down to the Top Stressed Processes table to identify the specific process by its application, behavior, name, and PID.

**Orange area growing in Stressed Processes.** Multiple processes are accumulating latency. This is broader than a single stuck process -- it suggests the node or cluster is under general pressure. Look at the Latency Distribution panel to understand how the latency is spread across ranges. Check the CPU panels to see if the system is compute-bound.

**A spike in Max Latency followed by a quick return to normal.** A temporary burst of load that the system recovered from. Compare the timing with the Process Spawn Rate panel (in the collapsed Processes row) to see if the spike correlates with a batch of new processes starting up or a restart event.

**Max Latency persistently elevated (minutes, not seconds).** A process is stuck. It has a message in its mailbox that arrived long ago and has not been processed. This is different from overload -- overload causes latency to fluctuate with traffic, while a stuck process shows a steadily increasing or flat high value. Identify the process in the Top Stressed Processes table and investigate its behavior.

#### Identify the node

The middle tier contains **Max Latency per Node** (left) and **Stressed Processes per Node** (right). These panels break down the cluster-wide values into individual nodes.

If one node shows high latency while others are calm, the problem is localized. That specific node may be overloaded, running a stuck process, or experiencing resource constraints. Cross-reference with the CPU, Memory, and Network panels for that node.

If multiple nodes show similar latency patterns, the problem is systemic. Common causes include a shared external dependency that is slow, a distributed traffic pattern that overwhelms multiple nodes simultaneously, or a deployment issue affecting the entire cluster.

The relationship between per-node max latency and per-node stressed count is informative. A node with high max latency but low stressed count has one problematic process while the rest are fine. A node with moderate latency but high stressed count is generally overloaded -- many processes are all slightly behind.

#### Understand the distribution

The **Latency Distribution** panel shows a stacked area chart where each layer represents a latency range. The color gradient runs from green (1ms, 5ms, 10ms) through yellow (50ms, 100ms) to orange (500ms, 1s) and red/dark-red (5s, 10s, 30s, 60s, 60s+). The legend is sorted from highest to lowest range.

In a healthy system, the chart is predominantly green or empty. Processes with latency under 10ms are operating normally -- the delay is within typical scheduling jitter.

Yellow layers (50ms-100ms) appearing during traffic spikes are acceptable if they disappear when the spike ends. Persistent yellow suggests the system is running near capacity.

Orange and red layers indicate processes that are seriously behind. Even a thin red sliver means some process has latency measured in seconds. Look at the Top Stressed Processes table to find it.

The distribution panel is particularly useful for distinguishing between two scenarios that look similar in the Max Latency panel: one stuck process (a single red sliver while the rest is green) versus widespread degradation (the entire chart shifting from green toward orange). The former requires investigating a specific process; the latter requires scaling or load-shedding.

#### Find the specific process

The **Top Stressed Processes** table at the bottom lists up to 50 processes with the highest mailbox latency across the cluster. Columns include Application, Behavior, Name, PID, Node, and Latency (plus Kubernetes labels when running in a containerized environment). The table is sorted by latency in descending order.

This table directly answers "which process is the bottleneck?" The Application and Behavior columns tell you what kind of actor it is. The Name column gives its registered name if it has one. The Node column tells you where it runs.

Multiple entries from the same application suggest that application is under pressure as a whole. A single entry with extreme latency (minutes) while others are in milliseconds points to a stuck or blocked process that needs specific investigation -- see the [Debugging](../../advanced/debugging.md) section for techniques to identify what a stuck process is doing.

#### Correlate with other panels

Latency data becomes most useful when combined with other dashboard panels:

- **High latency + high CPU** -- processes are compute-bound. The actors are doing heavy work in their callbacks and cannot keep up with the message rate. Consider distributing work across more processes or offloading expensive computation.

- **High latency + low CPU** -- processes are blocked on something other than computation. Common causes: waiting for external I/O (database, HTTP calls), waiting for responses from other actors via synchronous calls, or contention on shared resources.

- **High latency + growing memory** -- mailboxes are accumulating messages faster than processes can handle them. The unprocessed messages consume memory. If this continues, the node will eventually run out of memory.

- **High latency + network traffic spike** -- a burst of remote messages is overwhelming the receiving processes. Check the Network per Node and Network Detail panels to identify which node-to-node link is responsible.

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
