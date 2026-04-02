# Pulse

Tracing in Ergo Framework records observations locally on each node. To see the complete picture of a trace spanning multiple nodes, you need to send those observations to an external system that assembles them. Pulse exports tracing observations to any OTLP-compatible backend (Grafana Tempo, Jaeger, OpenTelemetry Collector) over HTTP.

Pulse runs as an application on your node. It registers itself as a tracing exporter, receives observations from the framework, batches them, and periodically flushes them to the configured collector. Each node in your cluster runs its own Pulse instance pointing to the same collector, and the backend assembles cross-node traces automatically.

## Adding to Your Node

```go
import (
    "ergo.services/application/pulse"
    "ergo.services/ergo"
    "ergo.services/ergo/gen"
)

func main() {
    node, err := ergo.StartNode("mynode@localhost", gen.NodeOptions{
        Applications: []gen.ApplicationBehavior{
            pulse.CreateApp(pulse.Options{
                Endpoint: "tempo:4318",
                Insecure: true,
            }),
        },
    })
    if err != nil {
        panic(err)
    }
    node.Wait()
}
```

With this configuration, Pulse sends observations to `http://tempo:4318/v1/traces` using protobuf encoding. The node name (`mynode@localhost`) is used as the OTLP resource `service.name`, so the backend groups observations by node.

## Configuration

```go
pulse.Options{
    Endpoint:      "tempo:4318",           // collector address
    Insecure:      true,                   // HTTP instead of HTTPS
    Headers:       map[string]string{      // custom HTTP headers
        "Authorization": "Bearer <token>",
    },
    BatchSize:     512,                    // flush after N observations
    FlushInterval: 5 * time.Second,        // max time between flushes
    PoolSize:      3,                      // number of export workers
    ExportTimeout: 10 * time.Second,       // HTTP request timeout
    Flags:         gen.TracingFlagSend |   // which observations to receive
                   gen.TracingFlagReceive |
                   gen.TracingFlagProcs,
}
```

| Option | Default | Description |
|--------|---------|-------------|
| Endpoint | `localhost:4318` | OTLP collector `host:port`. Pulse appends `/v1/traces` automatically. |
| Insecure | `false` | Use plain HTTP. Set to `true` for local development or when TLS is terminated by a proxy. |
| Headers | none | Custom HTTP headers sent with every export request. Use for authentication tokens or routing headers. |
| BatchSize | `512` | Maximum number of observations in a batch. When the batch reaches this size, it is flushed immediately. |
| FlushInterval | `5s` | Maximum time between flushes. Even if the batch is not full, it is flushed after this interval. |
| PoolSize | `3` | Number of export workers. Each worker maintains its own HTTP client and batch buffer. Increase if your observation rate exceeds what three workers can export. |
| ExportTimeout | `10s` | HTTP request timeout per flush. If the collector doesn't respond within this time, the flush fails and the error is logged. |
| Flags | Send + Receive + Procs | Which observation types Pulse receives. By default, Pulse receives everything. Set a subset to reduce volume, for example `TracingFlagSend` to export only Sent observations. |

## How It Works

Pulse starts a pool of worker actors. The pool registers itself as a process-based tracing exporter on the node. When the framework emits an observation matching the configured flags, it delivers the observation to the pool, which distributes it to a worker.

Each worker maintains a batch buffer. Observations accumulate until either the batch reaches `BatchSize` or `FlushInterval` elapses, whichever comes first. On flush, the worker converts the batch to OTLP protobuf format and sends it via HTTP POST to the collector.

Each worker has its own HTTP client with persistent connections. Workers operate independently. If one worker's flush is slow (waiting on the network), others continue batching and flushing. This provides throughput resilience under variable network conditions.

If a flush fails (network error, collector down, non-2xx response), the error is logged and the worker continues with the next batch. Observations from the failed batch are lost. This is a deliberate trade-off: retrying failed batches would introduce unbounded memory growth and backpressure that could affect the node's primary workload.

On shutdown, each worker flushes any remaining observations before terminating.

## OTLP Span Mapping

Each Ergo observation becomes one OTLP span. The mapping is deterministic. Any node can compute the OTLP span ID for any observation without coordination.

### Span ID Encoding

The OTLP span ID encodes both the Ergo span ID and the observation point:

```
OTLP SpanID = ErgoSpanID << 2 | Point
```

Where Point is: Sent=1, Delivered=2, Processed=3.

This means the three observations for a single message (Sent, Delivered, Processed) have related but distinct OTLP span IDs. Given any one, you can compute the other two.

### Parent-Child Relationships

| Observation | OTLP Parent | Meaning |
|-------------|-------------|---------|
| Delivered | Sent of same message | "delivered after sent" |
| Processed | Delivered of same message | "processed after delivered" |
| Sent (with parent) | Processed of causing message | "sent because of processing that message" |
| Sent (root) | none | first message in trace |

This creates the chain: Sent -> Delivered -> Processed -> (next) Sent -> Delivered -> Processed, forming the waterfall visible in Tempo and Jaeger.

### Span Attributes

Every OTLP span includes framework attributes prefixed with `ergo.`:

- `ergo.node` : node where the observation was recorded
- `ergo.from` : sender process identity
- `ergo.to` : recipient identity
- `ergo.kind` : Send, Request, Response, Spawn, or Terminate
- `ergo.point` : Sent, Delivered, or Processed
- `ergo.behavior` : actor behavior type name
- `ergo.message` : message type name
- `ergo.ref` : call reference (for Request/Response correlation)

Custom attributes set by the process via `SetTracingAttribute` and `SetTracingSpanAttribute` are included as additional OTLP span attributes.

### Span Name

The OTLP span name is formatted as:

```
{behavior} {kind}.{point} {message}
```

For example: `OrderProcessor Send.Sent main.ReserveStock`.

### Span Kind Mapping

| Ergo Kind | OTLP SpanKind |
|-----------|---------------|
| Send | PRODUCER |
| Request | CLIENT |
| Response | SERVER |
| Spawn | INTERNAL |
| Terminate | INTERNAL |

## Inspecting Workers

Each Pulse worker exposes statistics through the standard inspection mechanism. In the Observer process list, find the Pulse worker processes and inspect them to see:

- `spans_received` : total observations received by this worker
- `spans_exported` : total observations successfully exported
- `export_errors` : total failed flush attempts
- `batch_size` : current batch length

These counters help diagnose export problems: if `export_errors` is growing, the collector may be unreachable or overloaded.

## Grafana Tempo Setup

A minimal Tempo configuration for local development:

```yaml
# tempo.yaml
server:
  http_listen_port: 3200

distributor:
  receivers:
    otlp:
      protocols:
        http:
          endpoint: "0.0.0.0:4318"

storage:
  trace:
    backend: local
    local:
      path: /var/tempo/traces
    wal:
      path: /var/tempo/wal
```

Point Pulse at `tempo:4318` with `Insecure: true`. In Grafana, add Tempo as a data source (`http://tempo:3200`) and use the Explore view to search for traces by trace ID or attributes.
