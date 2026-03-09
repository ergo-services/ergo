<h1><a href="https://ergo.services"><img src=".github/images/logo.svg" alt="Ergo Framework" width="159" height="49"></a></h1>

[![Gitbook Documentation](https://img.shields.io/badge/GitBook-Documentation-f37f40?style=plastic&logo=gitbook&logoColor=white&style=flat)](https://docs.ergo.services)
[![GoDoc](https://pkg.go.dev/badge/ergo-services/ergo)](https://pkg.go.dev/ergo.services/ergo)
[![MIT license](https://img.shields.io/badge/license-MIT-brightgreen.svg)](https://opensource.org/licenses/MIT)
[![Telegram Community](https://img.shields.io/badge/Telegram-ergo__services-229ed9?style=flat&logo=telegram&logoColor=white)](https://t.me/ergo_services)
[![Reddit](https://img.shields.io/badge/Reddit-r/ergo__services-ff4500?style=plastic&logo=reddit&logoColor=white&style=flat)](https://reddit.com/r/ergo_services)

The Ergo Framework is an implementation of ideas, technologies, and design patterns from the Erlang world in the Go programming language. It is based on the actor model, network transparency, and a set of ready-to-use components for development. This significantly simplifies the creation of complex and distributed solutions while maintaining a high level of reliability and performance.

### Features ###

1. **Actor Model**: isolated processes communicate through message passing, handling messages sequentially in their own mailbox with four priority queues. Processes support both asynchronous messaging and synchronous request-response patterns, enabling flexible communication while maintaining the actor model guarantees.

2. **Network Transparency**: actors interact the same way whether local or remote. The framework uses custom serialization and protocol for [efficient](https://github.com/ergo-services/benchmarks) distributed communication with connection pooling, compression, and type caching, making network location transparent to application code.

3. **Supervision Trees**: hierarchical fault recovery where supervisors monitor child processes and apply restart strategies when failures occur. Supports multiple supervision types (One For One, All For One, Rest For One, Simple One For One) and restart strategies (Transient, Temporary, Permanent) for building self-healing systems.

4. **Meta Processes**: bridge blocking I/O with the actor model through dedicated meta processes that handle TCP, UDP, Port, and Web protocols. Meta processes run blocking operations without affecting regular actor message processing.

5. **Distributed Systems**: service discovery through embedded or external registrars (etcd, Saturn), distributed publish/subscribe events with token-based authorization and buffering, remote process spawning with factory-based permissions, and remote application orchestration across nodes.

6. **Ready-to-use Components**: core framework includes Actor, Supervisor, Pool, and WebWorker actors plus TCP, UDP, Port, and Web meta processes. Extra library provides Leader, Metrics actors, WebSocket, SSE meta processes, Observer application, Colored and Rotate loggers, Erlang protocol support.

7. **Flexibility**: customize network stack components, certificate management, compression and message priorities, logging, distributed events, and meta processes. Framework supports mTLS, NAT traversal, important delivery for guaranteed messaging, and Cron-based scheduling.

Examples demonstrating the framework's capabilities are available in the [examples repository](https://github.com/ergo-services/examples).

### Benchmarks ###

On a 64-core processor, Ergo Framework demonstrates a performance of **over 21 million messages per second locally** and **nearly 5 million messages per second over the network**.

![image](https://github.com/user-attachments/assets/813900ad-ff79-4cc7-b1e6-396c5781acb1)

Available benchmarks can be found in the [benchmarks repository](https://github.com/ergo-services/benchmarks).

* Messaging performance (local, network)

* Memory consumption per process (demonstrates framework memory footprint)

* Serialization performance comparison: EDF vs Protobuf vs Gob

* Distributed Pub/Sub (event delivery to 1,000,000 subscribers across 10 nodes)

### Observer ###
To inspect the node, network stack, running applications, and processes, you can use the [`observer`](https://github.com/ergo-services/tools/) tool

<img src="https://github.com/user-attachments/assets/1cb83305-6c56-4eb7-b567-76f3c551c176" width="500">

To install the Observer tool, you need to have the Go compiler version 1.20 or higher. Run the following command:

```
$ go install ergo.tools/observer@latest
```

You can also embed the [Observer application](https://docs.ergo.services/extra-library/applications/observer) into your node. To see it in action, see the [demo example](https://github.com/ergo-services/examples/tree/master/demo). For more information, visit the [Observer documentation](https://docs.ergo.services/tools/observer). 



### Quick start ###

For a quick start, use the [`ergo`](https://docs.ergo.services/tools/ergo) tool — a command-line utility designed to simplify the process of generating boilerplate code for your project based on the Ergo Framework. With this tool, you can rapidly create a complete project structure, including applications, actors, supervisors, network components, and more. It offers a set of arguments that allow you to customize the project according to specific requirements, ensuring it is ready for immediate development.

To install use the following command:

```
$ go install ergo.tools/ergo@latest
```

Now, you can create your project with just one command. Here is example:

Supervision tree

```
  mynode
  ├─ myapp
  │  │
  │  └─ mysup
  │     │
  │     └─ myactor
  ├─ myweb
  └─ myactor2
```

To generate project for this design use the following command:

```
$ ergo -init MyNode \
      -with-app MyApp \
      -with-sup MyApp:MySup \
      -with-actor MySup:MyActor \
      -with-web MyWeb \
      -with-actor MyActor2 \
      -with-observer 
```

as a result you will get generated project:

```
  mynode
  ├── apps
  │  └── myapp
  │     ├── myactor.go
  │     ├── myapp.go
  │     └── mysup.go
  ├── cmd
  │  ├── myactor2.go
  │  ├── mynode.go
  │  ├── myweb.go
  │  └── myweb_worker.go
  ├── go.mod
  ├── go.sum
  └── README.md
```

to try it:

```
$ cd mynode
$ go run ./cmd
```

Since we included Observer application, open http://localhost:9911 to inspect your node and running processes.

### Erlang support ###

Starting from version 3.0.0, support for the Erlang network stack has been moved to a [separate module](https://github.com/ergo-services/proto). Version 3.0 was distributed under the BSL 1.1 license, but starting from version 3.1 it is available under the MIT license. Detailed information is available in the [Erlang protocol documentation](https://docs.ergo.services/extra-library/network-protocols/erlang).

### Requirements ###

* Go 1.20.x and above

### Changelog ###

Fully detailed changelog see in the [ChangeLog](CHANGELOG.md) file.

#### [v3.3.0](https://github.com/ergo-services/ergo/releases/tag/v1.999.330) 2026-xx-xx [tag version v1.999.330] ####

* Added **pointer type support** in EDF - `*int`, `*string`, `[]*T`, `map[K]*V`, pointer struct fields. Nil state preserved. Nested pointers (`**T`) not supported. Max encoding depth limit (100) prevents stack overflow on deeply nested structures. See [Network Transparency](https://docs.ergo.services/networking/network-transparency) documentation
* Fixed logger to preserve Behavior name when process registers name

* Added **process lifecycle counters** to `NodeInfo` - `ProcessesSpawned`, `ProcessesSpawnFailed`, `ProcessesTerminated` for cumulative statistics

* Added **mailbox latency measurement** (build with `-tags=latency`). `QueueMPSC.Latency()` returns the age of the oldest message in the queue (nanoseconds), -1 if disabled. `ProcessMailbox.Latency()` returns the max across all four queues. Added `MailboxLatency` field to `ProcessShortInfo` and `MailboxQueues` latency fields to `ProcessInfo`. Added `Node.ProcessRangeShortInfo` for efficient iteration over all processes. See [actor/metrics](https://github.com/ergo-services/actor-metrics) for Prometheus integration with histogram, top-N, and Grafana dashboard

* Added **per-event metrics** - `EventInfo` now includes `MessagesPublished`, `MessagesLocalSent`, `MessagesRemoteSent` counters. Added `Node.EventInfo` and `Node.EventRangeInfo` for querying event statistics. Added `EventsPublished`, `EventsReceived`, `EventsLocalSent`, `EventsRemoteSent` to `NodeInfo`. `EventsPublished` counts only local producer publishes, `EventsReceived` counts events arriving from remote nodes

* Added **process init time measurement** - `InitTime` field in `ProcessShortInfo` and `ProcessInfo` records the time spent in `ProcessInit` callback (nanoseconds). Enables detection of slow process initialization

* Fixed **message counters for meta processes** - meta process traffic now propagates to parent process counters (`messagesIn`/`messagesOut`), making `ProcessRangeShortInfo` aggregates balanced. Meta process own counters preserved for meta-level observability

* Fixed **self-send message counter** - `messagesOut` now incremented for self-sends (process sending to itself), consistent with other send paths

* Fixed **simultaneous connect dead loop** - two nodes dialing each other at the same time no longer cause infinite retry loops. Deterministic connection IDs and Erlang-style collision detection (`EnableSimultaneousConnect` flag) ensure exactly one connection per pair. Fixed related connection leaks

* Fixed **silent data loss on connection pool write failure** - a transient write error could permanently break a pool item's write path without detection, causing all subsequent messages to be silently dropped while the connection appeared healthy

* Added **software keepalive** for inter-node connections - application-level heartbeat that detects silent failures invisible to TCP keepalive. Enabled by default (15s period, 3 misses, 45s timeout). See [Network Stack](https://docs.ergo.services/networking/network-stack#software-keepalive) documentation

* Added **handshake deadline** (5s) to prevent hung handshakes from blocking connection goroutines indefinitely

### Development and debugging ###

To enable Golang profiler just add `--tags pprof` in your `go run` or `go build` (profiler runs at
`http://localhost:9009/debug/pprof`). Use `PPROF_HOST` and `PPROF_PORT` environment variables to customize the address.

With `--tags pprof`, each actor goroutine is labeled with its PID and each meta process with its Alias for easy identification in pprof output:

```
curl -s "http://localhost:9009/debug/pprof/goroutine?debug=1" | grep -B5 'labels:.*pid'
curl -s "http://localhost:9009/debug/pprof/goroutine?debug=1" | grep -B5 'labels:.*meta'
```

Output:
```
1 @ 0x100c17fa0 ...
# labels: {"pid":"<ABC123.0.1005>"}
#   main.(*Worker).HandleMessage+0x27  /path/worker.go:45
```

This helps identify stuck processes during shutdown by matching PIDs/Aliases from the shutdown log with goroutine stack traces.

To disable panic recovery use `--tags norecover`.

To enable mailbox latency measurement use `--tags latency`. This adds a monotonic timestamp to every message pushed into the MPSC queue, allowing `QueueMPSC.Latency()` and `ProcessMailbox.Latency()` to report the age of the oldest unprocessed message. Overhead is approximately 10-25% on micro-benchmarks (LOCAL 1-1 scenario). Without the tag, `Latency()` returns -1 and there is zero overhead.

To enable trace logging level for the internals (node, network,...) use `--tags trace` and set the log level `gen.LogLevelTrace` for your node.

For detailed debugging techniques, troubleshooting scenarios, and best practices, see the [Debugging](https://docs.ergo.services/advanced/debugging) documentation.

To run tests with cleaned test cache:

```
go vet
go clean -testcache
go test -v ./testing/tests/...
```

### Commercial support

please, contact support@ergo.services for more information
