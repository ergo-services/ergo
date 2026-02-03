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

#### [v3.2.0](https://github.com/ergo-services/ergo/releases/tag/v1.999.320) 2026-02-04 [tag version v1.999.320] ####

* Introduced **mTLS support** - new `gen.CertAuthManager` interface for mutual TLS with CA pool management (`ClientCAs`, `RootCAs`, `ClientAuth`, `ServerName`). See [Mutual TLS](https://docs.ergo.services/networking/mutual-tls) documentation
* Introduced **NAT support** - new `RouteHost` and `RoutePort` options in `gen.AcceptorOptions` for nodes behind NAT or load balancers. See [Behind the NAT](https://docs.ergo.services/networking/behind-the-nat) documentation
* Introduced **spawn time control** - `InitTimeout` option in `gen.ProcessOptions` limits `ProcessInit` duration for both local and remote spawn. Remote spawn and application processes limited to max 15 seconds. See [Process](https://docs.ergo.services/basics/process) documentation
* Introduced **zip-bomb protection** - decompression size limits to prevent memory exhaustion attacks
* Added `gen.Ref` methods for request timeout tracking. See [Generic Types](https://docs.ergo.services/basics/generic-types#gen.ref):
  - `Deadline` - returns deadline timestamp stored in reference
  - `IsAlive` - checks if reference is still valid (deadline not exceeded)
* Added `gen.Node` methods. See [Node](https://docs.ergo.services/basics/node) documentation:
  - `ProcessPID` / `ProcessName` - resolve process PID by name and vice versa
  - `Call`, `CallWithTimeout`, `CallWithPriority`, `CallImportant`, `CallPID`, `CallProcessID`, `CallAlias` - synchronous requests from Node interface
  - `Inspect` / `InspectMeta` - inspect processes and meta processes
  - `MakeRefWithDeadline` - create reference with embedded deadline
* Added `gen.RemoteNode.ApplicationInfo` - query application information from remote nodes. See [Remote Start Application](https://docs.ergo.services/networking/remote-start-application) documentation
* Added `gen.Process` methods. See [Process](https://docs.ergo.services/basics/process) documentation:
  - `SendWithPriorityAfter` - delayed send with priority
  - `SendExitAfter` / `SendExitMetaAfter` - delayed exit signals
  - `SendResponseImportant` / `SendResponseErrorImportant` - important delivery for responses
* Added `gen.Meta` methods. See [Meta Process](https://docs.ergo.services/basics/meta-process) documentation:
  - `SendResponse` / `SendResponseError` - respond to requests from meta process
  - `SendPriority` / `SetSendPriority` - message priority control
  - `Compression` / `SetCompression` - compression settings
  - `EnvDefault` - get environment variable with default value
* Added `gen.ApplicationSpec` / `gen.ApplicationInfo` fields:
  - `Tags` - labels for instance selection (blue/green, canary, maintenance). See [Tags for Instance Selection](https://docs.ergo.services/basics/application#tags-for-instance-selection)
  - `Map` - logical role to process name mapping. See [Process Role Mapping](https://docs.ergo.services/basics/application#process-role-mapping)
* Added **HandleInspect** implementations for all supervisor types (OFO, ARFO, SOFO)
* Fixed **LinkChild** in `RemoteNode.Spawn` / `RemoteNode.SpawnRegister`
* Fixed **args persistence** for Simple One For One supervisor - child processes now restart with their original spawn arguments
* Fixed **critical bug**: terminate signals (Link/Monitor exits) were incorrectly rejected due to wrong incarnation validation in network layer. Thanks to [@qjpcpu](https://github.com/qjpcpu) for reporting [#248](https://github.com/ergo-services/ergo/issues/248)
* Completely reworked internal **Target Manager** (`node/tm/`) - improved architecture for process, event, and node target management with comprehensive test coverage
* Completely reworked internal **Pub/Sub** mechanism - improved reliability and performance
* Improved **ProcessInit state** - more `gen.Process` methods now available during initialization:
  - `Link*`, `Unlink*`, `Monitor*`, `Demonitor*`
  - `Call*`, `Inspect`, `InspectMeta`
  - `RegisterName`, `UnregisterName`, `RegisterEvent`, `UnregisterEvent`
  - `SendResponse*`, `SendResponseError*`
  - `CreateAlias`, `DeleteAlias`
* Introduced **shutdown timeout** - `ShutdownTimeout` option in `gen.NodeOptions` (default 3 minutes). During graceful shutdown, pending processes are logged every 5 seconds with state and queue info. After timeout, node force exits with error code 1. See [Node](https://docs.ergo.services/basics/node) documentation
* Added **pprof labels** for actor and meta process goroutines (with `--tags pprof`) - each process goroutine is labeled with its PID, each meta process with its Alias, making it easy to identify stuck processes in pprof output
* Improved API documentation - comprehensive godoc comments for all public interfaces
* **Documentation rewritten** - complete documentation now included in the repository (`docs/`) and available at [docs.ergo.services](https://docs.ergo.services)
* New documentation articles:
  - [Project Structure](https://docs.ergo.services/basics/project-structure) - organizing projects with message isolation levels, deployment patterns, and evolution strategies
  - [Building a Cluster](https://docs.ergo.services/advanced/building-a-cluster) - step-by-step guide to distributed systems with service discovery, load balancing, and failover
  - [Message Versioning](https://docs.ergo.services/advanced/message-versioning) - evolving message contracts in distributed clusters with explicit versioning strategies
  - [Handle Sync](https://docs.ergo.services/advanced/handle-sync) - synchronous message handling patterns
  - [Important Delivery](https://docs.ergo.services/advanced/important-delivery) - guaranteed delivery mechanism
  - [Pub/Sub Internals](https://docs.ergo.services/advanced/pub-sub-internals) - event system architecture
  - [Debugging](https://docs.ergo.services/advanced/debugging) - build tags, pprof integration, troubleshooting stuck processes

* **Extra Library - Actors** (https://github.com/ergo-services/actor):
  - Introduced **Leader** actor - distributed leader election with Raft-inspired consensus algorithm. Features: term-based disambiguation, automatic failover, split-brain prevention through majority quorum, dynamic peer discovery. See [documentation](https://docs.ergo.services/extra-library/actors/leader)
  - Introduced **Metrics** actor - Prometheus metrics exporter that collects node/network telemetry via HTTP endpoint. Features: automatic collection of node metrics (uptime, processes, memory), network metrics per remote node, extensible for custom metrics. See [documentation](https://docs.ergo.services/extra-library/actors/metrics)

* **Extra Library - Meta Processes** (https://github.com/ergo-services/meta):
  - Introduced **SSE** (Server-Sent Events) meta-process - unidirectional server-to-client streaming over HTTP. Features: server handler for accepting connections, client connection for external SSE endpoints, full SSE spec support (event types, IDs, retry hints, multi-line data), process pool with round-robin load balancing, Last-Event-ID for reconnection. See [documentation](https://docs.ergo.services/extra-library/meta-processes/sse)

* **Benchmarks** (https://github.com/ergo-services/benchmarks):
  - Introduced **Distributed Pub/Sub** benchmark - demonstrates event delivery to 1,000,000 subscribers across 10 nodes. Achieves 2.9M msg/sec delivery rate with only 10 network messages (one per consumer node) instead of 1M


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
