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
                URL: "http://tempo:4318/v1/traces",
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
    URL:           "http://tempo:4318/v1/traces", // full collector URL
    Headers:       map[string]string{             // custom HTTP headers
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
| URL | `http://localhost:4318/v1/traces` | Full OTLP/HTTP collector URL. |
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
| Sent (with parent) | Processed of causing message | "sent because of processing that message" |
| Sent (root) | none | first message in trace |
| Delivered | Sent of same message | "delivered after sent" |
| Processed | Sent of same message | "processed after sent" |
| Terminate.Processed | Processed of parent context | "process terminated" (no Sent for Terminate) |

Sent is the anchor for each message. Delivered and Processed are its children at the same level. Response spans nest under Request.Processed, forming a natural call hierarchy:

```
Req.Sent
├── Req.Delivered
└── Req.Processed
    └── Resp.Sent
        └── Resp.Delivered
```

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

The OTLP SpanKind depends on both the Ergo kind and the observation point:

| Ergo Kind + Point | OTLP SpanKind |
|-------------------|---------------|
| Send.Sent | PRODUCER |
| Send.Delivered | CONSUMER |
| Send.Processed | CONSUMER |
| Request.Sent | CLIENT |
| Request.Delivered | SERVER |
| Request.Processed | SERVER |
| Response.Sent | SERVER |
| Response.Delivered | CLIENT |
| Response.Processed | SERVER |
| Spawn | INTERNAL |
| Terminate | INTERNAL |

The Sent side of a message gets the initiator kind (CLIENT/PRODUCER), while the Delivered/Processed side gets the handler kind (SERVER/CONSUMER). For Response, the roles are inverted: Sent is SERVER (handler sending back), Delivered is CLIENT (caller receiving the answer).

## Reading Traces in Grafana

OTLP was designed for request-response services where a span represents a unit of work with a start and end time. Ergo's actor model is different: messages are instantaneous events (sent, delivered, processed), not duration-based operations. Pulse maps each event to a zero-duration OTLP span placed at the exact timestamp when the event occurred.

In trace visualization tools (Grafana, Jaeger, Zipkin), these appear as dots on a timeline rather than bars. This is expected. The horizontal distance between dots shows actual timing, and the tree structure shows causality.

### Call (Request/Response)

```
Time ─────────────────────────────────────────────────────────►

Node A    ●                                              ●
          Req.Sent                                 Resp.Delivered
          (CLIENT)                                    (CLIENT)

Node B              ●              ●      ●
                Req.Delivered  Req.Processed  Resp.Sent
                  (SERVER)      (SERVER)     (SERVER)

          ├── network ──┤── handling ──┤     ├── network ──┤
```

- Req.Sent to Req.Delivered = network latency from A to B
- Req.Delivered to Req.Processed = time B spent handling the request
- Req.Processed to Resp.Sent = response creation time
- Resp.Sent to Resp.Delivered = network latency from B back to A

### Send (async)

```
Time ──────────────────────────────────────►

Node A    ●
          Send.Sent
          (PRODUCER)

Node B              ●              ●
                Send.Delivered  Send.Processed
                 (CONSUMER)     (CONSUMER)

          ├── network ──┤── handling ──┤
```

### Forward (multi-hop)

```
Time ──────────────────────────────────────────────────────────────────────►

Node A    ●                                                           ●
          Req.Sent                                              Resp.Delivered

Node B              ●           ●  ●
                Req.Delivered  Req.Processed
                                   Fwd.Sent

Node C                                    ●           ●  ●
                                      Fwd.Delivered  Fwd.Processed
                                                         Resp.Sent

          ├── network ──┤─ handling ─┤── network ──┤─ handling ─┤── network ──┤
```

For duration-based visualization with timing bars, use the Observer web UI which renders Ergo traces natively.

## Inspecting Workers

Each Pulse worker exposes statistics through the standard inspection mechanism. In the Observer process list, find the Pulse worker processes and inspect them to see:

- `spans_received` : total observations received by this worker
- `spans_exported` : total observations successfully exported
- `export_errors` : total failed flush attempts
- `batch_size` : current batch length

These counters help diagnose export problems: if `export_errors` is growing, the collector may be unreachable or overloaded.

## Grafana Dashboard

Pulse includes a ready-to-use Grafana dashboard for trace search. Import `grafana-tracing.json` from the Pulse module into your Grafana instance. During import, Grafana will ask you to select a Tempo datasource.

The dashboard provides a TraceQL filter for searching traces by node, behavior, message type, or any span attribute. Results include columns for service name, ergo.kind, ergo.behavior, and ergo.message. Click any Trace ID to open the full waterfall view.

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
