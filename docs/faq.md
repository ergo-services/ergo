---
description: Answers to the questions developers and AI assistants ask most often
---

# FAQ

## General

### What is Ergo Framework?

Ergo is an open-source Go framework for building concurrent and distributed systems using the actor model. It brings Erlang/OTP design patterns, including isolated processes, supervision trees, and network-transparent messaging, to Go with zero external dependencies.

### Is Ergo production-ready?

Yes. Ergo is used in production systems. It supports [mTLS](networking/mutual-tls.md), [NAT traversal](networking/behind-the-nat.md), graceful shutdown, panic recovery with stack traces, and has a comprehensive test suite. The framework has been in active development since 2019.

### What license is Ergo distributed under?

MIT License. Free to use in commercial projects without restrictions.

### What Go version is required?

Go 1.20 or higher. No other dependencies.

## Actor Model

### What is the actor model and why use it in Go?

The actor model is a concurrency paradigm where independent units (actors, also called processes) communicate exclusively through message passing. Each actor has private state and processes messages one at a time. No shared memory, no mutexes, no race conditions.

Go's goroutines and channels are powerful but don't enforce isolation. Goroutines can share memory, which requires manual synchronization. Ergo enforces the actor model guarantees: isolated state, message-only communication, and sequential processing per actor. See [Actor Model](basics/actor-model.md).

### How is an Ergo process different from a goroutine?

| | Goroutine | Ergo Process |
|---|---|---|
| Identity | No stable address | Has PID, addressable locally and remotely |
| State | Can share memory | Strictly private |
| Failure recovery | Manual | Automatic via supervision |
| Cross-node messaging | Not built in | Same API, transparent |
| Race conditions | Possible | Impossible within a process |

See [Process](basics/process.md) for details.

### How many processes can run on a single node?

Thousands to hundreds of thousands. Processes sleep when idle and consume no CPU. Memory footprint per process is minimal, comparable to a goroutine plus a small mailbox struct.

### Can actors communicate synchronously?

Yes. Ergo supports both async (`Send`) and sync (`Call`) patterns. `Call` blocks the calling process until a response arrives or a timeout occurs, while maintaining full actor model guarantees. See [Handling Sync Requests](advanced/handle-sync.md).

## Fault Tolerance

### What happens when an actor crashes?

Its supervisor detects the failure and applies a restart strategy:

- **One-For-One**: restart only the failed child
- **All-For-One**: restart all children when one fails
- **Rest-For-One**: restart the failed child and all children started after it
- **Simple-One-For-One**: identical children spawned dynamically at runtime, restart failed ones

Supervision trees are hierarchical. A failed subtree is isolated and recovered without affecting the rest of the system. See [Supervision Tree](basics/supervision-tree.md) and [Supervisor](actors/supervisor.md).

### Do I need to write retry logic?

No. Supervision handles process recovery automatically. For message delivery, use the [Important Delivery](advanced/important-delivery.md) flag for guaranteed delivery semantics. The sender receives an immediate error if the target doesn't exist, rather than a timeout.

### What happens if a remote node disconnects?

All processes that were monitoring or linked to processes on the disconnected node receive a notification (`MessageDownNode` or exit signal). Your actors handle this notification and decide how to respond: retry, failover, or graceful degradation. See [Links and Monitors](basics/links-and-monitors.md).

## Distributed Systems

### How do nodes find each other?

Through a registrar. Each node runs a minimal built-in registrar by default. Nodes on the same host discover each other automatically via localhost. For production clusters across multiple hosts, configure an external registrar:

- **etcd**: distributed key-value store, widely used
- **Saturn**: Ergo's own central registrar, purpose-built for Ergo clusters

See [Service Discovering](networking/service-discovering.md).

### Do I need Kubernetes or a service mesh?

No. Ergo eliminates the integration tax of traditional microservice architectures. No HTTP or gRPC endpoints to define between services, no sidecar proxies, no API gateways for internal routing. Process-to-process communication is direct through the framework's network layer.

Ergo does support Kubernetes for deployment. The [Health](extra-library/actors/health.md) actor provides liveness, readiness, and startup health probes, and the [Metrics](extra-library/actors/metrics.md) actor provides Prometheus metrics on a single port.

### How does Ergo handle network partitions?

The [Leader](extra-library/actors/leader.md) actor uses a Raft-inspired consensus algorithm with majority quorum to prevent split-brain scenarios. When a partition occurs, only the partition with a majority of nodes continues to elect a leader. Minority partitions stop processing leader-dependent operations until connectivity is restored.

### Can I run Ergo nodes across different clouds?

Yes. [ergo.cloud](https://ergo.cloud) is a managed overlay network that connects Ergo nodes across AWS, GCP, Azure, and bare metal into one transparent cluster without VPNs, proxies, or tunnels. End-to-end encrypted. Currently available via waitlist.

## Pub/Sub

### How does distributed Pub/Sub work in Ergo?

A producer process registers a named event. Any process on any node subscribes using `LinkEvent` or `MonitorEvent`. The framework delivers messages to all subscribers transparently across the cluster.

```go
// Producer
token, _ := producer.RegisterEvent("market.prices", gen.EventOptions{})
producer.SendEvent("market.prices", token, PriceUpdate{Asset: "BTC", Price: 95000})

// Subscriber on any node
process.MonitorEvent(gen.Event{Name: "market.prices", Node: "producer@host"})

// Event messages arrive in HandleEvent
func (s *Sub) HandleEvent(message gen.MessageEvent) error {
    update := message.Message.(PriceUpdate)
    // handle update
    return nil
}

// Producer termination or event unregister arrives in HandleMessage as MessageDownEvent
func (s *Sub) HandleMessage(from gen.PID, msg any) error {
    switch msg.(type) {
    case gen.MessageDownEvent:
        // producer terminated or unregistered
    }
    return nil
}
```

See [Events](basics/events.md).

### How does Ergo Pub/Sub scale?

The framework uses fan-out at the consumer node level, not per subscriber. One network message is sent per remote node regardless of how many subscribers that node has. Local delivery then fans out within the node.

Result: 2.9M messages/second delivery rate to 1,000,000 subscribers across 10 nodes using only 10 network messages, not 1,000,000. See [Pub/Sub Internals](advanced/pub-sub-internals.md).

### What's the difference between Links, Monitors, and Events?

All three use the same underlying pub/sub mechanism internally. All three are unidirectional: the notification flows from the target to the watcher, not the other way around. Note this differs from Erlang, where links are bidirectional.

- **Link**: when the target terminates, the watcher receives an exit signal on its Urgent queue. The default behavior is to terminate the watcher. Actors can enable exit trapping to receive the signal as a `gen.MessageExit*` message and decide how to react.
- **Monitor**: when the target terminates, the watcher receives a `gen.MessageDown*` notification on its System queue. The watcher continues running.
- **Event**: the watcher subscribes to a named stream of messages published by a producer. The producer terminating also delivers a notification (exit signal for link-based subscriptions, down message for monitor-based).

See [Links and Monitors](basics/links-and-monitors.md) and [Pub/Sub Internals](advanced/pub-sub-internals.md).

## Performance

### How fast is Ergo?

- 21M+ messages/second locally on a 64-core processor
- ~5.5M messages/second over the network
- EDF serialization: up to 47% faster encoding than Protobuf, 6 to 14 times faster than Gob
- Distributed Pub/Sub: 2.9M msg/sec to 1M subscribers across 10 nodes

Full benchmarks: [benchmarks repository](https://github.com/ergo-services/benchmarks).

### How does Ergo serialization compare to Protobuf?

Ergo uses EDF (Ergo Data Format) with type caching. For repeated message types, type metadata is cached after the first transmission. Subsequent messages of the same type skip type information entirely. This makes EDF significantly faster than Protobuf for encoding and decoding in high-throughput scenarios.

## Observability

### Does Ergo support distributed tracing?

Yes. Ergo has native distributed tracing that follows message chains across processes and nodes. When a traced process sends a message, the trace identity travels with the message and propagates automatically through the entire downstream chain of handlers. You configure tracing on entry-point processes. Downstream actors need no instrumentation.

Traces can be viewed directly in Observer as waterfall diagrams or exported to OTLP-compatible backends (Grafana Tempo, Jaeger, OpenTelemetry Collector) via the [Pulse application](extra-library/applications/pulse.md). See [Distributed Tracing](advanced/distributed-tracing.md) for details.

### How do I inspect a running node?

Run the [Observer](extra-library/applications/observer.md) web UI for live visibility into processes, applications, network connections, events, logs, tracing waterfalls, and heap profiles. For AI-driven investigation, use the [MCP application](extra-library/applications/mcp.md) to expose the running system to Claude Code, Cursor, or any MCP-compatible client. For continuous metrics, the [Radar](extra-library/applications/radar.md) application provides a Prometheus endpoint with a ready-to-use Grafana dashboard.

## Integration

### Can Ergo nodes talk to Erlang/Elixir nodes?

Yes. Ergo supports the full Erlang network stack: EPMD, ETF (External Term Format), and DIST protocol. You can build hybrid Go/Erlang clusters where Ergo nodes and BEAM nodes coexist and communicate natively. See [Erlang protocol](extra-library/network-protocols/erlang.md).

### Does Ergo work with Prometheus and Grafana?

Yes. The [Metrics](extra-library/actors/metrics.md) actor exports node and network telemetry via a Prometheus HTTP endpoint. A ready-to-use Grafana dashboard is provided via [Radar](extra-library/applications/radar.md).

### Does Ergo support WebSockets and SSE?

Yes, via [Meta Processes](basics/meta-process.md). Each [WebSocket](extra-library/meta-processes/websocket.md) or [SSE](extra-library/meta-processes/sse.md) connection becomes an independent meta-process with a stable identifier (`gen.Alias`). Any actor anywhere in the cluster can send messages directly to a specific client connection. No routing intermediaries needed. This enables real-time push from any cluster node to any specific connected client.

### Can I use Ergo with standard Go HTTP libraries?

Yes. Ergo's [Web](meta-processes/web.md) meta-process integrates with standard `net/http`. You use any Go router (stdlib ServeMux, gorilla/mux, chi, echo) and any HTTP middleware. Actors are an implementation detail invisible to the HTTP layer.

## AI and MCP

### Can Ergo be used for AI agent infrastructure?

Yes, and it is particularly well-suited. Each AI agent runs as an isolated process with a mailbox. No shared state between agents, no race conditions. Supervisor trees restart stuck or crashed agents automatically. Multiple agents coordinate through message passing. Agents distribute transparently across cluster nodes as load grows. See [AI Agents](ai-agents.md) for patterns and diagnostics.

### What is MCP support in Ergo?

Ergo has built-in support for the Model Context Protocol (MCP), an emerging standard for AI tool integration. The [MCP application](extra-library/applications/mcp.md) exposes the running cluster to AI assistants (Claude Code, Cursor, and any MCP-compatible client) as a set of diagnostic tools. The AI inspects processes, queries events, captures goroutine dumps, reads logs, and runs samplers through natural language.

Two deployment modes:

- **Entry point**: the node runs an HTTP listener that accepts MCP requests. This is the node your AI client connects to.
- **Agent**: no HTTP listener. Accessible via cluster proxy from the entry point node. Use this for internal nodes that should be inspectable without exposing an HTTP port.

## Getting Started

### How do I create my first Ergo project?

```
# Install the project generator
go install ergo.tools/ergo@latest

# Create a project
ergo init MyNode github.com/myorg/mynode
cd mynode

# Add components
ergo add supervisor MyNodeApp:MySup
ergo add actor MySup:MyWorker

# Run
go run ./cmd
```

See [ergo tool documentation](tools/ergo.md) for the full command reference.

### Where can I get help?

- [Documentation](https://docs.ergo.services)
- [Examples](https://github.com/ergo-services/examples)
- [Telegram community](https://t.me/ergo_services)
- [GitHub Discussions](https://github.com/ergo-services/ergo/discussions)
- Commercial support: support@ergo.services
