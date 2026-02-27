# Radar

Running an Ergo node in production typically requires two things: health probes for Kubernetes and a Prometheus metrics endpoint. Setting them up separately means two HTTP servers on two ports, two actor packages to import, and the same wiring code repeated on every node.

Radar bundles both into a single application on one HTTP port. Internally it runs a [Health](../actors/health.md) actor for probe endpoints, a [Metrics](../actors/metrics.md) actor for base Ergo telemetry, and a pool of metrics workers for custom metric updates -- all behind a shared mux served by one HTTP server. Actors interact with Radar through helper functions in the `radar` package without importing the underlying packages or knowing the internal actor names.

## Adding to Your Node

```go
import (
    "ergo.services/application/radar"
    "ergo.services/ergo"
    "ergo.services/ergo/gen"
)

func main() {
    node, _ := ergo.StartNode("mynode@localhost", gen.NodeOptions{
        Applications: []gen.ApplicationBehavior{
            radar.CreateApp(radar.Options{Port: 9090}),
        },
    })

    // Health:  http://localhost:9090/health/live
    //          http://localhost:9090/health/ready
    //          http://localhost:9090/health/startup
    // Metrics: http://localhost:9090/metrics

    node.Wait()
}
```

With no signals registered, all three health endpoints return 200 with `{"status":"healthy"}`. The metrics endpoint immediately serves base Ergo metrics. No additional configuration is required for a working production setup.

## Configuration

```go
radar.Options{
    Host:                   "0.0.0.0",
    Port:                   9090,
    HealthPath:             "/health",
    MetricsPath:            "/metrics",
    HealthCheckInterval:    2 * time.Second,
    MetricsCollectInterval: 15 * time.Second,
    MetricsTopN:            100,
    MetricsPoolSize:        5,
}
```

**Host** determines which network interface the HTTP server binds to. Default is `"localhost"`. Use `"0.0.0.0"` for containerized environments where probes and scraping come from outside the pod.

**Port** sets the single HTTP port for all endpoints. Default is `9090`. Choose a port that does not conflict with your application's own listeners.

**HealthPath** sets the URL prefix for health probe endpoints. Default is `"/health"`. The actual endpoints become `HealthPath+"/live"`, `HealthPath+"/ready"`, `HealthPath+"/startup"`. Change this when deploying behind a reverse proxy that expects a different path prefix.

**MetricsPath** sets the URL path for the Prometheus scrape target. Default is `"/metrics"`.

**HealthCheckInterval** controls how often the health actor checks for expired heartbeats. Default is 1 second. Shorter intervals detect failures faster but increase internal message traffic. For most applications, 1-2 seconds provides a good balance.

**MetricsCollectInterval** sets how often base Ergo metrics are collected (processes, memory, CPU, network, events). Default is 10 seconds. Align this with your Prometheus scrape interval -- collecting more frequently than Prometheus scrapes wastes CPU; collecting less frequently means Prometheus may see stale values.

**MetricsTopN** limits the number of entries in per-process and per-event top-N metrics tables. Default is 50. Increase this for large nodes with thousands of processes where you need broader visibility into the tail. The collection cost scales linearly with TopN.

**MetricsPoolSize** sets the number of worker actors in the custom metrics pool. Default is 3. Under normal load, a single worker is sufficient. Increase this if many actors send frequent metric updates and you observe the metrics mailbox growing.

## Health Probes

Actors register signals with Radar, specifying which probes the signal affects and an optional heartbeat timeout. The health actor monitors the registering process -- if it terminates, all its signals are automatically marked as down.

### Registering a Signal

```go
func (w *DBWorker) Init(args ...any) error {
    radar.RegisterService(w, "postgres",
        radar.ProbeLiveness|radar.ProbeReadiness, 10*time.Second)

    w.scheduleHeartbeat()
    return nil
}

func (w *DBWorker) HandleMessage(from gen.PID, message any) error {
    switch message.(type) {
    case messageHeartbeat:
        radar.Heartbeat(w, "postgres")
        w.scheduleHeartbeat()
    }
    return nil
}

func (w *DBWorker) scheduleHeartbeat() {
    w.cancelHeartbeat, _ = w.SendAfter(w.PID(), messageHeartbeat{}, 3*time.Second)
}
```

The signal `"postgres"` participates in both liveness and readiness probes. If the heartbeat stops arriving (timeout expires) or the process terminates, Kubernetes receives a 503 on both `/health/live` and `/health/ready`.

### Probe Types

| Constant | Endpoint |
|----------|----------|
| `radar.ProbeLiveness` | `/health/live` |
| `radar.ProbeReadiness` | `/health/ready` |
| `radar.ProbeStartup` | `/health/startup` |

Combine with bitwise OR. A signal registered for `ProbeLiveness|ProbeReadiness` affects both endpoints independently.

### Manual Signal Control

When you can detect failures immediately without waiting for a timeout:

```go
case CacheConnectionLost:
    radar.ServiceDown(w, "cache")

case CacheConnectionRestored:
    radar.ServiceUp(w, "cache")
```

### Helper Functions

```go
radar.RegisterService(process, signal, probe, timeout)  // sync Call
radar.UnregisterService(process, signal)                 // sync Call
radar.Heartbeat(process, signal)                         // async Send
radar.ServiceUp(process, signal)                         // async Send
radar.ServiceDown(process, signal)                       // async Send
```

`RegisterService` and `UnregisterService` are synchronous calls that return an error on failure. `Heartbeat`, `ServiceUp`, and `ServiceDown` are asynchronous sends (fire-and-forget).

For a detailed explanation of the heartbeat model, failure detection mechanisms, and the HTTP response format, see the [Health](../actors/health.md) actor documentation.

## Custom Metrics

Actors register Prometheus metric collectors and update them through Radar's helper functions. The underlying metrics actor manages the Prometheus registry and HTTP exposition. Registration is synchronous, updates are asynchronous.

### Registering Metrics

```go
func (w *APIHandler) Init(args ...any) error {
    radar.RegisterGauge(w, "active_connections",
        "Number of active client connections", []string{"protocol"})

    radar.RegisterCounter(w, "requests_total",
        "Total HTTP requests processed", []string{"method", "status"})

    radar.RegisterHistogram(w, "request_duration_seconds",
        "Request latency distribution", []string{"method"},
        []float64{0.01, 0.05, 0.1, 0.5, 1.0, 5.0})

    return nil
}
```

The `labels` parameter defines the label names for the metric. When updating, you provide label values in the same order. Pass `nil` for metrics without labels. The `buckets` parameter in `RegisterHistogram` defines histogram bucket boundaries; pass `nil` for Prometheus default buckets.

### Updating Metrics

```go
func (w *APIHandler) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case RequestCompleted:
        radar.CounterAdd(w, "requests_total", 1,
            []string{msg.Method, msg.StatusCode})
        radar.HistogramObserve(w, "request_duration_seconds",
            msg.Duration.Seconds(), []string{msg.Method})
    case ConnectionChange:
        radar.GaugeSet(w, "active_connections",
            float64(msg.Count), []string{msg.Protocol})
    }
    return nil
}
```

Updates are distributed across the worker pool. Under high throughput, multiple actors can send updates concurrently without contending on a single actor's mailbox.

### Automatic Cleanup

When a process that registered metrics terminates, all its metrics are automatically unregistered from the Prometheus registry. No explicit cleanup is needed. To remove a metric while the process is still running, use `radar.UnregisterMetric(process, name)`.

### Helper Functions

```go
// Registration (sync Call, returns error)
radar.RegisterGauge(process, name, help, labels)
radar.RegisterCounter(process, name, help, labels)
radar.RegisterHistogram(process, name, help, labels, buckets)
radar.UnregisterMetric(process, name)

// Updates (async Send, fire-and-forget)
radar.GaugeSet(process, name, value, labels)
radar.GaugeAdd(process, name, value, labels)
radar.CounterAdd(process, name, value, labels)
radar.HistogramObserve(process, name, value, labels)
```

For a detailed explanation of metric types, the Grafana dashboard, and advanced usage (embedding, shared mode), see the [Metrics](../actors/metrics.md) actor documentation.

## Common Patterns

### Database Connection Pool

An actor that manages a connection pool reports both health and metrics through Radar:

```go
func (w *DBPool) Init(args ...any) error {
    // Health: liveness + readiness with heartbeat
    radar.RegisterService(w, "db_pool",
        radar.ProbeLiveness|radar.ProbeReadiness, 10*time.Second)

    // Metrics: connection pool gauge
    radar.RegisterGauge(w, "db_pool_connections",
        "Database connection pool size", []string{"state"})

    w.scheduleCheck()
    return nil
}

func (w *DBPool) HandleMessage(from gen.PID, message any) error {
    switch message.(type) {
    case messageCheck:
        if w.pool.Ping() == nil {
            radar.Heartbeat(w, "db_pool")
        }
        radar.GaugeSet(w, "db_pool_connections",
            float64(w.pool.ActiveCount()), []string{"active"})
        radar.GaugeSet(w, "db_pool_connections",
            float64(w.pool.IdleCount()), []string{"idle"})

        w.scheduleCheck()
    }
    return nil
}
```

A single periodic check updates both the health signal and connection pool metrics. If the database becomes unreachable, the heartbeat stops and Kubernetes removes the pod from service. The metrics endpoint continues to show the last known pool state until the pod restarts.

### Startup Gate with Progress

An actor that runs migrations uses the startup probe to prevent premature traffic, and reports progress via a gauge:

```go
func (w *Migrator) Init(args ...any) error {
    radar.RegisterService(w, "migrations", radar.ProbeStartup, 0)
    radar.RegisterGauge(w, "migrations_pending",
        "Number of pending migrations", nil)

    w.Send(w.PID(), messageRunMigrations{})
    return nil
}

func (w *Migrator) HandleMessage(from gen.PID, message any) error {
    switch message.(type) {
    case messageRunMigrations:
        pending := w.countPending()
        radar.GaugeSet(w, "migrations_pending", float64(pending), nil)

        if err := w.runNext(); err != nil {
            return err
        }

        if w.countPending() > 0 {
            w.Send(w.PID(), messageRunMigrations{})
            return nil
        }

        // All done -- mark startup complete
        radar.GaugeSet(w, "migrations_pending", 0, nil)
        radar.ServiceUp(w, "migrations")
        radar.UnregisterService(w, "migrations")
    }
    return nil
}
```

While migrations run, the startup probe returns 503, Kubernetes waits, and Prometheus shows the remaining migration count. Once complete, the startup signal is released and liveness/readiness probes take over.

## Kubernetes Configuration

Configure Kubernetes probes and Prometheus scraping to point at the same port:

```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
    - name: myapp
      livenessProbe:
        httpGet:
          path: /health/live
          port: 9090
        periodSeconds: 10
      readinessProbe:
        httpGet:
          path: /health/ready
          port: 9090
        periodSeconds: 10
      startupProbe:
        httpGet:
          path: /health/startup
          port: 9090
        failureThreshold: 30
        periodSeconds: 2
```

Prometheus scrape configuration:

```yaml
scrape_configs:
  - job_name: 'ergo'
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 15s
```

Align `scrape_interval` with `MetricsCollectInterval` in Radar options. The default collect interval is 10 seconds -- scraping more frequently than the collect interval returns identical data.

## Relationship to Health and Metrics Actors

Radar uses [Health](../actors/health.md) and [Metrics](../actors/metrics.md) actors internally. The helper functions in the `radar` package delegate to these actors by their internal registered names. If you need capabilities beyond what the helpers expose -- embedding the metrics actor for direct Prometheus registry access, custom health actor behavior with `HandleSignalDown` callbacks, or shared mux with additional HTTP handlers -- use the underlying actors directly.

Radar is designed for the common case: production nodes that need standard health probes and Prometheus metrics with minimal setup. For advanced scenarios, the building blocks are available as separate packages.
