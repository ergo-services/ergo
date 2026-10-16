---
description: Answers to the questions developers and AI assistants ask most often
---

# FAQ

## General

### What is Ergo Framework?

Ergo is an open-source Go framework for building concurrent and distributed systems using the actor model. It brings Erlang/OTP design patterns, including isolated processes, supervision trees, and network-transparent messaging, to Go with zero external dependencies.

### Is Ergo production-ready?

The runtime includes supervision, graceful shutdown, panic recovery and [mTLS](networking/mutual-tls.md). Whether an application is ready for production also depends on its message contracts, overload policy, state recovery and deployment. The Testing chapters cover actor behavior and tests across real node connections; the Advanced chapters cover diagnosis and cluster operation.

### What license is Ergo distributed under?

MIT License. See the repository's `LICENSE` file for its terms, including the notice that must accompany copies.

### What Go version is required?

The core module declares Go 1.21 and has no external module dependencies. Extra-library modules can require a newer Go version and their own dependencies; check the `go.mod` of the packages you use.

## Actor Model

### What is the actor model and why use it in Go?

The actor model is a concurrency paradigm where independent units (actors, also called processes) communicate exclusively through message passing. Each actor has private state and processes messages one at a time, so that state needs no mutex.

Go's goroutines and channels give you no structure for that: identity, supervision, and addressing across nodes are all yours to build. Ergo provides them - one goroutine and one mailbox per process, message-only communication, sequential handling.

What it does not do is take memory isolation out of your hands. A message between two processes on the same node is handed over as the Go value it is, with no copy: send a map, a slice or a pointer and both processes hold the same memory. Only crossing a node boundary encodes, and that is what copies. Send values, or treat a send as handing over ownership. See [Actor Model](basics/actor-model.md).

### How is an Ergo process different from a goroutine?

| | Goroutine | Ergo Process |
|---|---|---|
| Identity | No stable address | Has PID, addressable locally and remotely |
| State | Shared by default | Private by discipline: nothing else reaches it unless you send a reference |
| Failure recovery | Manual | Automatic via supervision |
| Cross-node messaging | Not built in | Same API, transparent |
| Race conditions | Possible | None on state the actor alone reaches - it handles one message at a time |

See [Process](basics/process.md) for details.

### How many processes can run on a single node?

There is no fixed process count suitable for every node. Capacity depends on the state each actor holds, queued messages and the work performed by handlers. Idle processes do not keep dispatching callbacks. Measure memory and latency with representative traffic before choosing a deployment size.

### Can actors communicate synchronously?

Yes. `Send` submits a message without waiting for an application response. `Call` waits for an answer or call error and prevents that process from dispatching other mailbox messages while it waits. `SendRequest` returns without waiting and delivers the answer through `HandleResponse`. See [Handling Sync Requests](advanced/handle-sync.md) and [Requests Without Blocking](advanced/async-request.md).

## Fault Tolerance

### What happens when an actor crashes?

Its supervisor detects the failure and applies a restart strategy:

- **One-For-One**: restart only the failed child
- **All-For-One**: restart all children when one fails
- **Rest-For-One**: restart the failed child and all children started after it
- **Simple-One-For-One**: identical children spawned dynamically at runtime, restart failed ones

A child failure can be handled within its supervisor, or propagate when a restart budget is exceeded. The chosen strategy may also restart siblings. A new actor begins with fresh behavior state unless the application explicitly restores it. See [Supervision Tree](basics/supervision-tree.md) and [Supervisor](actors/supervisor.md).

### Do I need to write retry logic?

For application work, usually yes. Supervision restarts a process according to its policy; it does not automatically repeat an external operation or recover the old actor's state. Important delivery reports a routing attempt, while a timeout can leave the result unknown. Define when to retry, how to recognize a duplicate, and where to obtain the result of work already completed. [Important Delivery](advanced/important-delivery.md) follows this distinction with a job-submission example.

### What happens if a remote node disconnects?

Everyone who was watching something over that connection is told, and the message matches what they were watching, not the fact that a node went away. A monitor on a remote **process** yields `MessageDownPID` or `MessageDownProcessID` with a reason; a link yields the corresponding exit signal. `MessageDownNode` and `MessageExitNode` arrive only for those who monitored or linked the **node itself** through `MonitorNode` / `LinkNode`, and they carry a name and no reason. Your actors handle the notification and decide how to respond: retry, failover, or graceful degradation. See [Links and Monitors](basics/links-and-monitors.md).

## Distributed Systems

### How do nodes find each other?

Through a registrar. Each node runs a minimal built-in registrar by default. Nodes on the same host discover each other automatically via localhost. For production clusters across multiple hosts, configure an external registrar:

- **etcd**: distributed key-value store, widely used
- **Saturn**: Ergo's own central registrar, purpose-built for Ergo clusters

See [Service Discovering](networking/service-discovering.md).

### Do I need Kubernetes or a service mesh?

No. There are no HTTP or gRPC endpoints to define between services, no sidecar proxies, no API gateways for internal routing. Process-to-process communication is direct through the framework's network layer.

Ergo does support Kubernetes for deployment. The [Health](extra-library/actors/health.md) actor provides liveness, readiness, and startup health probes, and the [Metrics](extra-library/actors/metrics.md) actor provides Prometheus metrics on a single port.

### How does Ergo handle network partitions?

The [Leader](extra-library/actors/leader.md) actor uses terms, votes and heartbeats without a replicated log. It requires a majority of each actor's current membership view. With a shared view, a partition prevents the minority from electing; after membership changes or independent discovery, different groups may elect separately. Use fencing or external confirmation for effects that require a single writer.

### Can I run Ergo nodes across different clouds?

Yes, if the nodes have reachable routes and compatible connection settings. Configure discovery or static routes and TLS for the deployment. A private address returned by discovery may be unreachable from another network; [Behind the NAT](networking/behind-the-nat.md) explains advertised addresses, and [Static Routes](networking/static-routes.md) explains overrides.

### How do I pass money amounts in messages?

Use `decimal.Decimal` from `ergo.services/ergo/lib/decimal`, not `float64`. It holds 36 significant digits exactly in 16 bytes, arithmetic does not allocate, and EDF encodes it in 2 to 18 bytes. The method set follows `github.com/shopspring/decimal`, so code using it moves over by changing the import path. See the [package documentation](https://pkg.go.dev/ergo.services/ergo/lib/decimal).

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
func (s *Sub) HandleEvent(event gen.MessageEvent) error {
    update := event.Message.(PriceUpdate)
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

This reduces network fan-out when many subscribers share a node. Each receiving node still has to enqueue the event for its local subscribers, so total delivery work grows with their number. See [Pub/Sub Internals](advanced/pub-sub-internals.md) for the routing path and the [benchmarks repository](https://github.com/ergo-services/benchmarks) for a million-subscriber workload.

### What's the difference between Links, Monitors, and Events?

All three are relations held by the same target manager inside the node, which is why they behave alike and why a link, a monitor and an event subscription are all torn down the same way. All three are unidirectional: the notification flows from the target to the watcher, not the other way around. Note this differs from Erlang, where links are bidirectional.

- **Link**: when the target terminates, the watcher receives an exit signal on its Urgent queue. The default behavior is to terminate the watcher. Actors can enable exit trapping to receive the signal as a `gen.MessageExit*` message and decide how to react.
- **Monitor**: when the target terminates, the watcher receives a `gen.MessageDown*` notification on its System queue. The watcher continues running.
- **Event**: the watcher subscribes to a named stream of messages published by a producer. The producer terminating also delivers a notification (exit signal for link-based subscriptions, down message for monitor-based).

See [Links and Monitors](basics/links-and-monitors.md) and [Pub/Sub Internals](advanced/pub-sub-internals.md).

## Performance

### How fast is Ergo?

Throughput depends on how many processes can work concurrently, message size, handler cost, mailbox limits, and whether delivery crosses a node connection. The [benchmarks repository](https://github.com/ergo-services/benchmarks) includes local and remote ping, serialization, memory usage, and distributed Pub/Sub workloads. Ping benchmarks now live in the framework's `testing/benchmarks/ping` directory and can be run with `make bench`.

Use these workloads to establish a baseline on your hardware, then measure your own handlers and messages. A transport benchmark with small messages does not predict the throughput of actors waiting on a database or external API.

### How does Ergo serialization compare to Protobuf?

EDF builds encoding and decoding functions at runtime when you register Go types; it still uses reflection to access values. During a handshake, peers exchange registered types and establish compact numeric references. A cached registered-type marker occupies three bytes: a tag plus a two-byte ID. Types registered after the handshake can be sent by their full canonical name until a later handshake establishes a cached reference. Field values follow the type contract, without repeating struct field names.

These choices affect wire size and encoding cost, but do not establish a universal speed advantage over Protobuf. Compare both formats with representative messages, and consider how each handles schema changes. See [Network Transparency](networking/network-transparency.md#edf-ergo-data-format) and the [serialization benchmarks](https://github.com/ergo-services/benchmarks#serialization-benchmarks-edf-vs-protobuf-vs-gob).

## Observability

### Does Ergo support distributed tracing?

Yes. Ergo has native distributed tracing that follows message chains across processes and nodes. When a traced process sends a message, the trace identity travels with the message and propagates automatically through the entire downstream chain of handlers. You configure tracing on entry-point processes. Downstream actors need no instrumentation.

Traces can be viewed directly in Observer as waterfall diagrams or exported to OTLP-compatible backends (Grafana Tempo, Jaeger, OpenTelemetry Collector) via the [Pulse application](extra-library/applications/pulse.md). See [Distributed Tracing](advanced/distributed-tracing.md) for details.

### How do I inspect a running node?

Run the [Observer](extra-library/applications/observer.md) web UI for live visibility into processes, applications, network connections, events, logs, tracing waterfalls, and heap profiles. The same application serves an MCP surface, which exposes the running system to Claude Code, Cursor, or any MCP-compatible client for AI-driven investigation. For continuous metrics, the [Radar](extra-library/applications/radar.md) application provides a Prometheus endpoint with a ready-to-use Grafana dashboard.

### What can Observer do?

Observer is a live dashboard for a running Ergo system. You add it with one line of code and open `http://localhost:9911` in a browser. It shows you, updating every second, what your program is actually doing inside, and it lets you act on it without stopping it or adding any code.

An Ergo program is built from many small processes that each do one job and talk to each other by sending messages. Observer lets you:

- **See the whole node at a glance**: how much memory and CPU it uses, how many processes are running, how busy they are.
- **Find the part that is misbehaving**: sort and filter the process list to spot the one that is overloaded, stuck, or using the most time, even among tens of thousands of them, or color an application's process tree so the hot branch stands out.
- **Look inside a single process**: its message rates, what it is connected to, and any internal state it chooses to report.
- **Watch things happen in real time**: a filterable live log stream, the actual messages a producer is publishing as they go out, and a single request traced step by step as it travels from process to process and across machines.
- **Track down memory leaks and freezes**: live memory and goroutine views, including flame graphs, with no special build and no restart.

Observer is not read-only. From the same screen you can change a process's log level, send it a message, restart or stop it, start and stop parts of the application, and switch tracing on, all on the live system.

One Observer covers the whole cluster. Every Ergo node exposes itself to Observer automatically, so you run it on one node and move between any of them from the sidebar, reaching each through service discovery or by address.

See [Observer Application](extra-library/applications/observer.md) to add it to your node, and [Inspecting With Observer](advanced/observer.md) for a guided tour.

## Testing

### How do I test actors?

Ergo ships a dedicated testing harness. An actor is not a function you can call and inspect, so the harness observes it the way the rest of the system does: through what it does. Every outward action (a message sent, a process spawned, a log line written, an exit signal, a timer scheduled) is captured as a record. You drive the actor with inputs and assert on the records it produced, testing behavior rather than reaching into private state.

The harness has four layers that share one fluent assertion grammar:

| Package | Use it for |
|---------|------------|
| `unit` | One actor in-process against a mock node. Synchronous, deterministic, fast. Where most actor logic is tested. |
| `stage` | Real nodes running real actors over the real network, including multi-node clusters. For the scheduler, supervision, restarts, links and monitors across nodes, remote spawn, and disconnects. |
| `mock` | Standalone fakes of the `gen.*` interfaces to inject into ordinary non-actor code (helpers, resolvers, constructors). |
| `check` | The shared record types and `Should...` grammar the other layers expose. |

```go
sub, _ := unit.Spawn(t, factoryWorker, gen.ProcessOptions{})
sub.SendMessage(client, StartJob{ID: "42"})
sub.ShouldSend().To(gen.Atom("scheduler")).Message(JobQueued{ID: "42"}).Once().Assert()
```

See [Testing Overview](testing/overview.md).

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

Yes. Each AI agent runs as an isolated process with a mailbox, handling one message at a time. Supervisor trees restart stuck or crashed agents automatically. Multiple agents coordinate through message passing. Agents distribute transparently across cluster nodes as load grows. See [AI Agents](ai-agents.md) for patterns and diagnostics.

### What is MCP support in Ergo?

Ergo has built-in support for the Model Context Protocol (MCP), an emerging standard for AI tool integration. The [Observer](extra-library/applications/observer.md) application serves it beside its web UI: the running cluster reaches AI assistants (Claude Code, Cursor, and any MCP-compatible client) as resources to read and tools to call. The AI inspects processes, queries events, captures goroutine dumps, reads logs, and asks the same question of every node, through natural language.

There is one deployment shape, and only one node needs anything installed. Add the observer to a single node - that node serves `/mcp` and is what your AI client connects to. Every other node is reachable through it as it is, because each already runs the built-in `system` application the observer asks: a resource is addressed as `ergo://<node>/<lens>`, and every tool takes a `node` argument. Nothing has to be opened on the nodes being inspected, and the observer's own node holds no privileged position - name it explicitly like any other.

What a listener exposes is configurable per listener: the UI, the API and the MCP surface can each be disabled, and a read-only ceiling refuses the mutating tools. See [Observer Application](extra-library/applications/observer.md).

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
