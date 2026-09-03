# Changelog
All notable changes to this project will be documented in this file.

This format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

#### [v3.3.0](https://github.com/ergo-services/ergo/releases/tag/v1.999.330) 2026-09-04 [tag version v1.999.330] ####

* Added **distributed tracing** - a trace context propagates across `Send`, `Call`, spawn, and every hop a message takes between nodes, with configurable sampling and cross-node clock-skew correction. Enable with `NetworkFlags.EnableTracing` (plus `NetworkFlags.EnableClockSkew` for skew correction). The new `ergo.services/application/pulse` application exports the collected spans over OTLP. See [Distributed Tracing](https://docs.ergo.services/advanced/distributed-tracing) documentation
* Added **EDF schema evolution** - off by default; when `NetworkFlags.EnableSchemaEvolution` is enabled on both nodes, fields appended to the end of a registered struct stay wire-compatible: a peer that does not know the new trailing fields skips them, and a peer missing them zero-fills. By default EDF remains strict and any contract change requires a new package version. A single evolvable struct is bounded at just under 4GB. See [Message Versioning](https://docs.ergo.services/advanced/message-versioning) documentation
* Added a **testing framework** under `ergo.services/ergo/testing` with four layers sharing one fluent assertion grammar: `unit` (in-process single-actor testing against a mock node), `stage` (live multi-node integration), `mock` (standalone `gen.*` mocks), and `check` (the shared assertion core). Covers positive and negative expectations, per-value matching, and observation of messages, calls, spawns, links, monitors, and events. See [Testing](https://docs.ergo.services/testing/overview) documentation
* Added **open events** - `EventOptions.Open` disables the token check on `SendEvent`, so any local process may publish to an event by name without holding the token returned by `RegisterEvent` (the token is still issued, just not required). `UnregisterEvent` remains restricted to the producer. Off by default. See [Events](https://docs.ergo.services/basics/events) documentation
* Added **pointer type support** in EDF - `*int`, `*string`, `[]*T`, `map[K]*V`, pointer struct fields. Nil state preserved. Nested pointers (`**T`) not supported. Max encoding depth limit (100) prevents stack overflow on deeply nested structures. See [Network Transparency](https://docs.ergo.services/networking/network-transparency) documentation
* Added **per-type encode/decode statistics** (build with `-tags=typestats`) - tracks the count of root-level encode/decode operations and decompressed wire-byte volume per registered EDF type, exposed via `Network().RegisteredTypes()` and the Observer Types panel. Helps identify heavy message types and where compression is worth enabling. Overhead is approximately 2-3% on encode/decode throughput; zero without the tag
* Added **software keepalive** for inter-node connections. Application-level heartbeat detects silent failures that TCP keepalive cannot: stuck processes, broken flushers, goroutine starvation. Each side advertises its period during handshake (8 bits in `NetworkFlags`); receiver uses peer's period for timeout. Enabled by default (15s period, 3 misses, 45s timeout). Configure via `NetworkFlags.EnableSoftwareKeepAlive` (the keepalive period in seconds; 0 disables it) and `NetworkOptions.SoftwareKeepAliveMisses`. See [Network Stack](https://docs.ergo.services/networking/network-stack#software-keepalive) documentation
* Added **handshake deadline** (5s) to prevent hung handshakes from blocking connection goroutines indefinitely
* Added **message fragmentation** for large messages. Messages exceeding the fragment size (default 65000 bytes) are automatically split for transmission and reassembled on the receiving side. Works with compression, important delivery, and all message types. With `KeepNetworkOrder` disabled, fragments are distributed across all TCP connections in the pool for maximum throughput. Both nodes must enable `EnableFragmentation` flag (enabled by default). Configure via `NetworkOptions.FragmentSize`, `FragmentTimeout`, `MaxFragmentAssemblies`. See [Network Stack](https://docs.ergo.services/networking/network-stack#message-fragmentation) documentation
* Added **`gen.NodeShortInfo`** and **`Node.ShortInfo()`** - the cheap counterpart of `NodeInfo` for polling every node of a cluster: identity, process and application counters, delivery error counters, log counters, runtime memory and GC figures, the names of the loaded applications, and the node's current connections. Memory and GC values are sampled through `runtime/metrics`, so no stop-the-world pause is involved
* Added **`gen.RemoteNodeShortInfo`** - the short form of `RemoteNodeInfo` carried by `NodeShortInfo.Peers`: peer name, connection age, message and byte counters, reconnections and TLS. Polling a cluster for the full `RemoteNodeInfo` of every connection costs seven times the memory for data a topology view never shows
* Added **`Process.ShortInfo()`** - `gen.ProcessShortInfo` for the calling process, previously reachable only from the node level through `ProcessListShortInfo` / `ProcessRangeShortInfo`
* Added **node short info inspector** to the system application - `inspect.RequestInspectNodeShort` answers with the first snapshot immediately and then publishes `MessageInspectNodeShort` every 3 seconds while somebody is subscribed. Nothing runs on a node nobody watches
* Added **`Registrar.Nodes()` and `Registrar.Event()` support to the embedded registrar** - `Nodes()` lists the other nodes registered on this host plus those registered on the hosts of connected peers (cached, self excluded), and `Event()` reports `gen.MessageRegistrarNodeJoined` / `NodeLeft` for this host. See [Service Discovering](https://docs.ergo.services/networking/service-discovering) documentation
* Added **`Peers()`** to the `gen.NodeRegistrar` bridge interface, letting a registrar see the nodes its node is connected with
* Removed the **stop-the-world pause from `Node.Info()`** - memory figures now come from `runtime/metrics` instead of `runtime.ReadMemStats`. The `MemoryUsed` and `MemoryAlloc` field comments were also wrong and are corrected: they hold the memory obtained from the OS and the memory occupied by live heap objects
* Removed the **stop-the-world pause from the heap inspector** - it called `runtime.ReadMemStats` on every tick, once per second, on any node with the Observer profiler open. The goroutine dump now sizes its buffer from the goroutine count as well, so a capture no longer repeats `runtime.Stack` (and its pause) while the buffer grows
* Added **process lifecycle counters** to `gen.NodeInfo` - `ProcessesSpawned`, `ProcessesSpawnFailed`, `ProcessesTerminated` for cumulative statistics
* Added **mailbox latency measurement** (build with `-tags=latency`). `QueueMPSC.Latency()` returns the age of the oldest message in the queue (nanoseconds), -1 if disabled. `ProcessMailbox.Latency()` returns the max across all four queues. Added `MailboxLatency` field to `ProcessShortInfo` and latency fields to `MailboxQueues` in `ProcessInfo`. See [Debugging](https://docs.ergo.services/advanced/debugging) documentation
* Added **`Node.ProcessRangeShortInfo`** for efficient callback-based iteration over all processes with their current state
* Added **per-event metrics** - `EventInfo` now includes `MessagesPublished`, `MessagesLocalSent`, `MessagesRemoteSent` counters. Added `Node.EventInfo` and `Node.EventRangeInfo` for querying event statistics. Added `EventsPublished`, `EventsReceived`, `EventsLocalSent`, `EventsRemoteSent` to `NodeInfo`. `EventsPublished` counts only local producer publishes, `EventsReceived` counts events arriving from remote nodes
* Added **process init time measurement** - `InitTime` field in `ProcessShortInfo` and `ProcessInfo` records the time spent in `ProcessInit` callback (nanoseconds)
* Added per-process activity fields to `ProcessShortInfo` and `ProcessInfo` - `RunningTime` (cumulative nanoseconds in the Running state), `StateTime` (nanoseconds since the process entered its current state), and `Wakeups` (cumulative count of wake-ups)
* Added `ServerTime` to `gen.NodeInfo` - the node's current wall-clock time with timezone
* Added `Reconnections` to `gen.RemoteNodeInfo` - total number of connection-pool item reconnections
* Added **`Network().ResolveApplication`** - a shortcut for `Network().Registrar().Resolver().ResolveApplication(name)` that returns the same `gen.ApplicationRoutes` and reports the same error as `Registrar()` when no registrar is configured. See [Service Discovering](https://docs.ergo.services/networking/service-discovering) documentation
* Added **heap object counters to `gen.NodeInfo`** - `HeapAllocObjects` and `HeapFreeObjects`, the cumulative number of heap objects allocated and freed, sampled through `runtime/metrics` with no stop-the-world pause. Two readings give the allocation and collection rates over the interval, and the difference between the counters is the number of objects currently alive, so a client can draw GC pressure from the node info stream instead of walking the whole memory profile for it
* Added **`ApplicationInfo.ProcessesTotal`** - how many processes belong to the application, the group members and everything they spawned together. `Group` still lists the declared members only; the new counter is the whole set the application's teardown waits for
* Fixed **application teardown** - the `Terminate` callback of an application now runs after every process of that application has terminated, its own `Terminate` callback included, instead of right after the last group member left the process table. Closing a shared resource there is therefore safe: the processes of the application can use it to the end of their own `Terminate`. Along with it: processes spawned outside the group are stopped by the teardown instead of outliving the application; a group member terminating while the group is still being spawned now applies the application mode, so a permanent application no longer reaches `Running` with a member missing; `ApplicationStop` returns once the whole teardown is done and the application is already stopped; `node.Stop()` waits for the `ProcessTerminate` callbacks and for the application teardown before taking the network and the loggers down. See [Application](https://docs.ergo.services/basics/application) documentation
* Fixed logger to preserve Behavior name when process registers name
* Fixed **simultaneous connect dead loop** - two nodes dialing each other at the same time no longer cause infinite retry loops. Deterministic connection IDs and Erlang-style collision detection (`EnableSimultaneousConnect` flag) ensure exactly one connection per pair. Fixed related connection leaks
* Fixed **silent data loss on connection pool write failure** - a transient write error could permanently break a pool item's write path without detection, causing all subsequent messages to be silently dropped while the connection appeared healthy
* Fixed **important delivery use-after-release** - reference ID for acknowledgment was read from buffer after it was returned to the pool, causing corrupted ACK responses under load. Affected `SendImportant` for PID, ProcessID, and Alias targets
* Fixed **dropped fast replies** in the EDF network protocol - a sync reply arriving before the caller began waiting could be lost; result channels are now buffered. Thanks to [@JeroenSoeters](https://github.com/JeroenSoeters) for the fix [#259](https://github.com/ergo-services/ergo/pull/259)
* Fixed **`MarshalEDF` length-prefix corruption** - a custom `MarshalEDF` implementation that produced enough output to grow the encode buffer could have its length prefix written to the wrong location, corrupting the encoded value. Thanks to [@JeroenSoeters](https://github.com/JeroenSoeters) for the fix [#257](https://github.com/ergo-services/ergo/pull/257)
* Fixed **message counters for meta processes** - meta process traffic now propagates to parent process counters, making `ProcessRangeShortInfo` aggregates balanced
* Fixed **self-send message counter** - `messagesOut` now incremented for self-sends
* Fixed **meta process alias collision** - `MakeRef` kept only the low 18 and the top 18 bits of its counter, dropping the 28 bits in between, so a ref repeated every 262144 allocations. Spawning a meta process registered its alias with no occupancy check, so on such a repeat it silently took over the entry of a live meta: messages addressed to the older one were delivered to the newer one, and once the newer one closed, its teardown removed the shared entry and left the older one unreachable with `ErrProcessUnknown` while its connection was still open. The counter is now carried in full and `SpawnMeta` returns `gen.ErrTaken` instead of overwriting. Thanks to [@bilus](https://github.com/bilus) for reporting [#265](https://github.com/ergo-services/ergo/issues/265)

**Deprecated**

* Deprecated the package-level EDF registration helpers - `edf.RegisterTypeOf`, `edf.RegisterTypesOf`, `edf.RegisterError` and `edf.RegisterAtom` now print a deprecation warning when called directly. The canonical way is to register through the node - `node.Network().RegisterType`, `RegisterTypes`, `RegisterError` and `RegisterAtom`. See [Network Transparency](https://docs.ergo.services/networking/network-transparency) documentation

**Extra library**

* **Saturn** (`ergo.services/registrar/saturn` and `ergo.tools/saturn`) changed from Business Source License 1.1 to **MIT**. The central registrar and its client library are now free for production and commercial use, with no licence to purchase and no node cap. Every module of the ecosystem is MIT.

**Applications**

* Reworked the **Observer application** (`ergo.services/application/observer`) into a full cluster inspector. The embedded web UI (live updates over SSE) now shows node info with a live memory graph; the full process list with per-process state, mailbox depth, message latency, running time and wakeups, plus a detail view (supervision tree, links, monitors, names, aliases, environment, and live `HandleInspect` state); meta-processes; applications (start/stop/unload); the network stack (connections, routes, and the wire-type registry with inferred schemas); events; a live log stream; and on-demand goroutine and heap profiles with no special build or restart. Beyond read-only it can send messages, send exits, kill processes, change log levels and adjust per-process network settings. Because every node runs the built-in `system` application, one Observer instance can switch to and inspect any node in the cluster without deploying anything there. The same application also serves an **MCP endpoint** for AI agents: 38 tools and 13 resource lenses over the live cluster, 26 of the tools read-only, with capability ceilings that can only narrow and refusals that name the capability. A node built with `DisableManage` never starts the mutating plane at all. See [Observer](https://docs.ergo.services/extra-library/applications/observer) and [MCP](https://docs.ergo.services/advanced/mcp) documentation
* Added the **Pulse application** (`ergo.services/application/pulse`) - an OTLP/HTTP exporter that ships the node's distributed-tracing spans to a collector (Grafana Tempo, Jaeger, etc.) with batching, a worker pool, configurable span kinds and custom headers for authentication. Ships a Grafana tracing dashboard. See [Pulse](https://docs.ergo.services/extra-library/applications/pulse) documentation
* Added the **Radar application** (`ergo.services/application/radar`) - one drop-in application serving both Kubernetes health probes (`/health/live`, `/ready`, `/startup`) and a Prometheus `/metrics` endpoint on a single port. It wraps the `health` and `metrics` actors behind package-level helpers, so any actor can register liveness/readiness signals and send heartbeats, or register gauges/counters/histograms and top-N metrics, without importing the underlying actors. See [Radar](https://docs.ergo.services/extra-library/applications/radar) documentation

**Actors**

* Added the **Health actor** (`ergo.services/actor/health`) - serves Kubernetes liveness/readiness/startup probes. Actors register named signals (a bitmask of probes) and send heartbeats; a missed heartbeat or the registering process terminating marks the signal down and the probe returns 503. Runs its own HTTP server or registers on a shared mux. See [Health actor](https://docs.ergo.services/extra-library/actors/health) documentation
* Expanded the **Metrics actor** (`ergo.services/actor/metrics`) well beyond the basic node/network telemetry from 3.2.0: top-N processes by mailbox depth, message latency, running time, throughput in/out, wakeups and drain ratio; mailbox-latency histograms (`-tags=latency`); process lifecycle counters (spawned / spawn-failed / terminated); per-process init time, wakeups, mailbox depth and utilization; event metrics (subscribers, published, local/remote-sent top-N, events received); log-messaging and handshake/connection-churn metrics; per-node Prometheus labels and CPU-core count; and a custom top-N API. Ships a complete Grafana cluster dashboard. See [Metrics actor](https://docs.ergo.services/extra-library/actors/metrics) documentation
* Fixed a panic in the **Leader actor** (`ergo.services/actor/leader`) - a self-Join into a nil map, by skipping the local PID in Join and guarding vote-reply handling

**Loggers**

* Added the **Sentry logger** (`ergo.services/logger/sentry`) - a `gen.Logger` that forwards framework panics and errors to a Sentry project with stack traces captured at the panic site. Node/network/application errors are forwarded by default; meta and process errors are opt-in. Configurable DSN (or `$SENTRY_DSN`), environment, release, bounded queue and flush timeout. See [Sentry logger](https://docs.ergo.services/extra-library/loggers/sentry) documentation
* Improved the **colored** and **rotate** loggers - both now render application-sourced log messages (previously unhandled); the rotate logger gains an `IncludeFields` option and meta-process behavior output, plus a colored-logger padding fix and a rotate field-leak fix

**Tools**

* Reworked the **`ergo` code generator** (`ergo.tools/ergo`) - generated scaffolding is now split into `*_gen.go` files (regenerated on every `ergo generate`, marked `DO NOT EDIT`) and `*_user.go` files (created once, safe to hand-edit), so re-running the generator after editing `ergo.yaml` no longer overwrites your code

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


#### [v3.1.0](https://github.com/ergo-services/ergo/releases/tag/v1.999.310) 2025-09-04 [tag version v1.999.310] ####

**New Features**
- **Cron Scheduler**: New `gen.Cron` interface enables scheduling tasks with cron expressions, supporting second-level precision for precise task execution. See https://docs.ergo.services/basics/cron
- **Port Meta Process**: New `meta.Port` allows spawning and managing external OS processes with bidirectional communication through stdin/stdout/stderr. See https://docs.ergo.services/meta-processes/port, example https://github.com/ergo-services/examples/port
- **Unit Testing Framework**: Comprehensive testing library (`testing/unit`) provides isolated actor testing with event capture and validation capabilities. See https://docs.ergo.services/testing/unit

**Enhancements**
- **Enhanced Logging**: Default logger now supports JSON output format with structured fields, improving observability and log processing
- **Environment Management**: Added `gen.Process.EnvDefault()` and `gen.Node.EnvDefault()` methods
- **Logger Fields**: Added `gen.Log.PushFields()` and `gen.Log.PopFields()` for contextual logging
- **EDF Protocol**: Added support for `encoding.BinaryMarshaler/BinaryUnmarshaler` interfaces
- **Performance**: Multiple optimizations across message handling and network operations

**Critical Bug Fixes**
- **Node Shutdown**: Fixed race condition causing "close of closed channel" panic during graceful shutdown
- **Supervisor Issues**: Fixed OFO supervisor child termination (#213), restart intensity calculation with millisecond precision, and duplicate Terminate callbacks
- **SIGTERM Handling**: Improved graceful shutdown behavior and SOFO supervisor cleanup
- **EDF Codec**: Fixed nil slice/map decoding issues
- **Local Registrar**: Improved resolver detection for service discovery

**Extra Library**
- **Module Independence**: All extra library modules (Logger, Meta, Registrar, etc...) are now independent Go modules with dependency management
- **Tools Domain**: All tools moved to dedicated `ergo.tools` domain for better organization and distribution
- **Proto**: `erlang23` (Erlang network stack implementation) changed from BSL 1.1 to MIT license for broader adoption and commercial use
- **Registrar**: New etcd registrar implementation with distributed service discovery, hierarchical configuration, real-time cluster events. See https://docs.ergo.services/extra-library/registrars/etcd-client and example https://github.com/ergo-services/examples/docker
- **Logger**: Added LogField support in colored logger, banner functionality, and fixed options handling. See https://docs.ergo.services/extra-library/loggers
- **Application**: Observer application enhanced with new Applications page, Cron job details, and UI fixes. See https://docs.ergo.services/extra-library/applications/observer
- **Benchmarks**: New serialization benchmarks comparing EDF vs Gob vs Protobuf performance, expanded test suite coverage. See https://github.com/ergo-services/benchmarks

#### [v3.0.0](https://github.com/ergo-services/ergo/releases/tag/v1.999.300) 2024-09-04 [tag version v1.999.300] ####

This version marks a significant milestone in the evolution of the Ergo Framework. The framework's design has been completely overhauled, and this version was built from the ground up. It includes:

- Significant API Improvements: The `gen.Process`, `gen.Node`, and `gen.Network` interfaces have been enhanced with numerous convenient methods.
- A New Network Stack: This version introduces a completely new network stack for improved performance and flexibility. See https://github.com/ergo-services/benchmarks for the details

Alongside the release of Ergo Framework 3.0.0, new tools and an additional components library are also introduced:

- Tools (observer, saturn) https://github.com/ergo-services/tools
- Loggers (rotate, colored) - https://github.com/ergo-services/logger
- Meta (websocket) - https://github.com/ergo-services/meta
- Application (observer) - https://github.com/ergo-services/application
- Registrar (client Saturn) - https://github.com/ergo-services/registrar
- Proto (erlang23) - https://github.com/ergo-services/proto

Finally, we've published comprehensive documentation for the framework, providing detailed guides to assist you in leveraging all the capabilities of Ergo Framework effectively. Its available at https://docs.ergo.services.

#### [v2.2.4](https://github.com/ergo-services/ergo/releases/tag/v1.999.224) 2023-05-01 [tag version v1.999.224] ####

This release includes fixes:
- Fixed incorrect handling of `gen.SupervisorStrategyRestartTransient` restart strategy in `gen.Supervisor`
- Fixed missing `ServerBehavior` in [`gen.Pool`, `gen.Raft`, `gen.Saga`, `gen.Stage`, `gen.TCP`, `gen.UDP`, `gen.Web`] behavior interfaces
- Introduced the new tool for boilerplate code generation - `ergo` https://github.com/ergo-services/tools. You may read more information about this tool in our article with a great example https://blog.ergo.services/quick-start-1094d56d4e2

#### [v2.2.3](https://github.com/ergo-services/ergo/releases/tag/v1.999.223) 2023-04-02 [tag version v1.999.223] ####

This release includes fixes:
- Improved `gen.TCP`. Issue #152
- Fixed incorrect decoding registered map type using etf.RegisterType
- Fixed race condition on process termination. Issue #153

#### [v2.2.2](https://github.com/ergo-services/ergo/releases/tag/v1.999.222) 2023-03-01 [tag version v1.999.222] ####

* Introduced `gen.Pool`. This behavior implements a basic design pattern with a pool of workers. All messages/requests received by the pool process are forwarded to the workers using the "Round Robin" algorithm. The worker process is automatically restarting on termination. See example here [examples/genpool](https://github.com/ergo-services/examples/tree/master/genpool)
* Removed Erlang RPC support. A while ago Erlang has changed the way of handling this kind of request making this feature more similar to the regular `gen.Server`. So, there is no reason to keep supporting it. Use a regular way of messaging instead - `gen.Server`.
* Fixed issue #130 (`StartType` option in `gen.ApplicationSpec` is ignored for the autostarting applications)
* Fixed issue #143 (incorrect cleaning up the aliases belonging to the terminated process)

#### [v2.2.1](https://github.com/ergo-services/ergo/releases/tag/v1.999.221) 2023-01-18 [tag version v1.999.221] ####

* Now you can join your services made with Ergo Framework into a single cluster with transparent networking using our **Cloud Overlay Network** where they can connect to each other smoothly, no matter where they run - AWS, Azure or GCP, or anywhere else. All these connections are secured with end-to-end encryption. Read more in this article [https://https://medium.com/@ergo-services/cloud-overlay-network](https://https://medium.com/@ergo-services/cloud-overlay-network). Here is an example of this feature in action [examples/cloud](https://github.com/ergo-services/examples/tree/master/cloud)
* `examples` moved to https://github.com/ergo-services/examples
* Added support Erlang OTP/25
* Improved handling `nil` values for the registered types using `etf.RegisterType(...)`
* Improved self-signed certificate generation
* Introduced `ergo.debug` option that enables extended debug information for `lib.Log(...)`/`lib.Warning(...)`
* Fixed `gen.TCP` and `gen.UDP` (missing callbacks)
* Fixed ETF registering type with `etf.Pid`, `etf.Alias` or `etf.Ref` value types
* Fixed Cloud client
* Fixed #117 (incorrect hanshake process finalization)
* Fixed #139 (panic of the gen.Stage partition dispatcher)

#### [v2.2.0](https://github.com/ergo-services/ergo/releases/tag/v1.999.220) 2022-10-18 [tag version v1.999.220] ####

* Introduced `gen.Web` behavior. It implements **Web API Gateway pattern** is also sometimes known as the "Backend For Frontend" (BFF). See example [examples/genweb](https://github.com/ergo-services/examples/tree/master/genweb)
* Introduced `gen.TCP` behavior - **socket acceptor pool for TCP protocols**. It provides everything you need to accept TCP connections and process packets with a small code base and low latency. Here is simple example [examples/gentcp](https://github.com/ergo-services/examples/tree/master/gentcp)
* Introduced `gen.UDP` - the same as `gen.TCP`, but for UDP protocols. Example is here [examples/genudp](https://github.com/ergo-services/examples/tree/master/genudp)
* Introduced **Events**. This is a simple pub/sub feature within a node - any `gen.Process` can become a producer by registering a new event `gen.Event` using method `gen.Process.RegisterEvent`, while the others can subscribe to these events using `gen.Process.MonitorEvent`. Subscriber process will also receive `gen.MessageEventDown` if a producer process went down (terminated). This feature behaves in a monitor manner but only works within a node. You may also want to subscribe to a system event - `node.EventNetwork` to receive event notification on connect/disconnect any peers.
* Introduced **Cloud Client** - allows connecting to the cloud platform [https://ergo.sevices](https://ergo.services). You may want to register your email there, and we will inform you about the platform launch day
* Introduced **type registration** for the ETF encoding/decoding. This feature allows you to get rid of manually decoding with `etf.TermIntoStruct` for the receiving messages. Register your type using `etf.RegisterType(...)`, and you will be receiving messages in a native type
* Predefined set of errors has moved to the `lib` package
* Updated `gen.ServerBehavior.HandleDirect` method (got extra argument `etf.Ref` to distinguish the requests). This change allows you to handle these requests asynchronously using method `gen.ServerProcess.Reply(...)`
* Updated `node.Options`. Now it has field `Listeners` (type `node.Listener`). It allows you to start any number of listeners with custom options - `Port`, `TLS` settings, or custom `Handshake`/`Proto` interfaces
* Fixed build on 32-bit arch
* Fixed freezing on ARM arch #102
* Fixed problem with encoding negative int8
* Fixed #103 (there was an issue on interop with Elixir's GenStage)
* Fixed node stuck on start if it uses the name which is already taken in EPMD
* Fixed incorrect `gen.ProcessOptions.Context` handling


#### [v2.1.0](https://github.com/ergo-services/ergo/releases/tag/v1.999.210) 2022-04-19 [tag version v1.999.210] ####

* Introduced **compression feature** support. Here are new methods and options to manage this feature:
  - `gen.Process`:
    - `SetCompression(enable bool)`, `Compression() bool`
    - `SetCompressionLevel(level int) bool`, `CompressionLevel() int`
    - `SetCompressionThreshold(threshold int) bool`, `CompressionThreshold() int` messages smaller than the threshold will be sent with no compression. The default compression threshold is 1024 bytes.
  - `node.Options`:
    - `Compression` these settings are used as defaults for the spawning processes
  - this feature will be ignored if the receiver is running on either the Erlang or Elixir node
* Introduced **proxy feature** support **with end-to-end encryption**.
  - `node.Node` new methods:
    - `AddProxyRoute(...)`, `RemoveProxyRoute(...)`
    - `ProxyRoute(...)`, `ProxyRoutes()`
    - `NodesIndirect()` returns list of connected nodes via proxy connection
  - `node.Options`:
    - `Proxy` for configuring proxy settings
  - includes support (over the proxy connection): compression, fragmentation, link/monitor process, monitor node
  - example [examples/proxy](https://github.com/ergo-services/examples/tree/master/proxy).
  - this feature is not available for the Erlang/Elixir nodes
* Introduced **behavior `gen.Raft`**. It's improved implementation of [Raft consensus algorithm](https://raft.github.io). The key improvement is using quorum under the hood to manage the leader election process and make the Raft cluster more reliable. This implementation supports quorums of 3, 5, 7, 9, or 11 quorum members. Here is an example of this feature [examples/genraft](https://github.com/ergo-services/examples/tree/master/genraft).
* Introduced **interfaces to customize network layer**
  - `Resolver` to replace EPMD routines with your solution (e.g., ZooKeeper or any other service registrar)
  - `Handshake` allows customizing authorization/authentication process
  - `Proto` provides the way to implement proprietary protocols (e.g., IoT area)
* Other new features:
  - `gen.Process` new methods:
    - `NodeUptime()`, `NodeName()`, `NodeStop()`
  - `gen.ServerProcess` new method:
    - `MessageCounter()` shows how many messages have been handled by the `gen.Server` callbacks
  - `gen.ProcessOptions` new option:
    - `ProcessFallback` allows forward messages to the fallback process if the process mailbox is full. Forwarded messages are wrapped into `gen.MessageFallback` struct. Related to issue #96.
  - `gen.SupervisorChildSpec` and `gen.ApplicationChildSpec` got option `gen.ProcessOptions` to customize options for the spawning child processes.
* Improved sending messages by etf.Pid or etf.Alias: methods `gen.Process.Send`, `gen.ServerProcess.Cast`, `gen.ServerProcess.Call` now return `node.ErrProcessIncarnation` if a message is sending to the remote process of the previous incarnation (remote node has been restarted). Making monitor on a remote process of the previous incarnation triggers sending `gen.MessageDown` with reason `incarnation`.
* Introduced type `gen.EnvKey` for the environment variables
* All spawned processes now have the `node.EnvKeyNode` variable to get access to the `node.Node` value.
* **Improved performance** of local messaging (**up to 8 times** for some cases)
* **Important** `node.Options` has changed. Make sure to adjust your code.
* Fixed issue #89 (incorrect handling of Call requests)
* Fixed issues #87, #88 and #93 (closing network socket)
* Fixed issue #96 (silently drops message if process mailbox is full)
* Updated minimal requirement of Golang version to 1.17 (go.mod)
* We still keep the rule **Zero Dependencies**

#### [v2.0.0](https://github.com/ergo-services/ergo/releases/tag/v1.999.200) 2021-10-12 [tag version v1.999.200] ####

* Added support of Erlang/OTP 24 (including [Alias](https://blog.erlang.org/My-OTP-24-Highlights/#eep-53-process-aliases) feature and [Remote Spawn](https://blog.erlang.org/OTP-23-Highlights/#distributed-spawn-and-the-new-erpc-module) introduced in Erlang/OTP 23)
* **Important**: This release includes refined API (without backward compatibility) for a more convenient way to create OTP-designed microservices. Make sure to update your code.
* **Important**: Project repository has been moved to [https://github.com/ergo-services/ergo](https://github.com/ergo-services/ergo). It is still available on the old URL [https://github.com/halturin/ergo](https://github.com/halturin/ergo) and GitHub will redirect all requests to the new one (thanks to GitHub for this feature).
* Introduced new behavior `gen.Saga`. It implements Saga design pattern - a sequence of transactions that updates each service state and publishes the result (or cancels the transaction or triggers the next transaction step). `gen.Saga` also provides a feature of interim results (can be used as transaction progress or as a part of pipeline processing), time deadline (to limit transaction lifespan), two-phase commit (to make distributed transaction atomic). Here is example [examples/gensaga](https://github.com/ergo-services/examples/tree/master/gensaga).
* Introduced new methods `Process.Direct` and `Process.DirectWithTimeout` to make direct request to the actor (`gen.Server` or inherited object). If an actor has no implementation of `HandleDirect` callback it returns `ErrUnsupportedRequest` as a error.
* Introduced new callback `HandleDirect` in the `gen.Server` interface as a handler for requests made by `Process.Direct` or `Process.DirectWithTimeout`. It should be easy to interact with actors from outside.
* Introduced new types intended to be used to interact with Erlang/Elixir
  * `etf.ListImproper` to support improper lists like `[a|b]` (a cons cell).
  * `etf.String` (an alias for the Golang string) encodes as a binary in order to support Elixir string type (which is `binary()` type)
  * `etf.Charlist` (an alias for the Golang string) encodes as a list of chars `[]rune` in order to support Erlang string type (which is `charlist()` type)
* Introduced new methods `Node.ProvideRemoteSpawn`, `Node.RevokeRemoteSpawn`, `Process.RemoteSpawn`.
* Introduced new interfaces `Marshaler` (method `MarshalETF`) and `Unmarshaler` (method `UnmarshalETF`) for the custom encoding/decoding data.
* Improved performance for the local messaging (up to 3 times for some cases)
* Added example [examples/http](https://github.com/ergo-services/examples/tree/master/http) to demonsrate how HTTP server can be integrated into the Ergo node.
* Added example [examples/gendemo](https://github.com/ergo-services/examples/tree/master/gendemo) - how to create a custom behavior (design pattern) on top of the `gen.Server`. Take inspiration from the [gen/stage.go](gen/stage.go) or [gen/saga.go](gen/saga.go) design patterns.
* Added support FreeBSD, OpenBSD, NetBSD, DragonFly.
* Fixed RPC issue #45
* Fixed internal timer issue #48
* Fixed memory leaks #53
* Fixed double panic issue #52
* Fixed Atom Cache race conditioned issue #54
* Fixed ETF encoder issues #64 #66

#### [v1.2.0](https://github.com/ergo-services/ergo/releases/tag/v1.2.0) - 2021-04-07 [tag version v1.2.0] ####

* Added TLS support. Introduced new option `TLSmode` in `ergo.NodeOptions` with the following values:
  - `ergo.TLSmodeDisabled` default value. encryption is disabled
  - `ergo.TLSmodeAuto` enables encryption with autogenerated and self-signed certificate
  - `ergo.TLSmodeStrict` enables encryption with specified server/client certificates and keys
  there is example of usage `examples/nodetls/tlsGenServer.go`
* Introduced [GenStage](https://hexdocs.pm/gen_stage/GenStage.html) behavior implementation (originated from Elixir world).
  `GenStage` is an abstraction built on top of `GenServer` to provide a simple way to create a distributed Producer/Consumer architecture, while automatically managing the concept of backpressure. This implementation is fully compatible with Elixir's GenStage. Example here `examples/genstage` or just run it `go run ./examples/genstage` to see it in action
* Introduced new methods `AddStaticRoute`/`RemoveStaticRoute` for `Node`. This feature allows you to keep EPMD service behind a firewall.
* Introduced `SetTrapExit`/`TrapExit` methods for `Process` in order to control the trapping `gen.MessageExit` message (for the linked processes)
* Introduced `TermMapIntoStruct` and `TermProplistIntoStruct` functions. It should be easy now to transform `etf.Map` or `[]eft.ProplistElement` into the given struct. See documentation for the details.
* Improved DIST implementation in order to support KeepAlive messages and get rid of platform-dependent `syscall` usage
* Fixed `TermIntoStruct` function. There was a problem with `Tuple` value transforming into the given struct
* Fixed incorrect decoding atoms `true`, `false` into the booleans
* Fixed race condition and freeze of connection serving in corner case [#21](https://github.com/ergo-services/ergo/issues/21)
* Fixed problem with monitoring process by the registered name (local and remote)
* Fixed issue with termination linked processes
* Fixed platform-dependent issues. Now Ergo Framework has tested and confirmed support of Linux, MacOS, Windows.

#### [v1.1.0](https://github.com/ergo-services/ergo/releases/tag/v1.1.0) - 2020-04-23 [tag version v1.1.0] ####

* Fragmentation support (which was introduced in Erlang/OTP 22)
* Completely rewritten network subsystem (DIST/ETF).
* Improved performance in terms of network messaging (outperforms original Erlang/OTP up to x5 times. See [Benchmarks](#benchmarks))

#### [v1.0.0](https://github.com/ergo-services/ergo/releases/tag/1.0.0) - 2020-03-03 [tag version 1.0.0] ####

* We have changed the name - Ergo (or Ergo Framework). GitHub's repo has been
renamed as well. We also created cloned repo `ergonode` to support users of
the old version of this project. So, its still available at
https://github.com/halturin/ergonode. But it's strongly recommend to use
the new one.
* Completely reworked (almost from scratch) architecture whole project
* Implemented linking process feature (in order to support Application/Supervisor behaviors)
* Reworked Monitor-feature. Now it has full-featured support with remote process/nodes
* Added multinode support
* Added experimental observer support
* Fixed incorrect ETF string encoding
* Improved ETF TermIntoStruct decoder
* Improved code structure and readability

#### [v0.2.0](https://github.com/ergo-services/ergo/releases/tag/0.2.0) - 2019-02-23 [tag version 0.2.0] ####
* Now we make versioning releases
* Improve node creation. Now you can specify the listening port range. See 'Usage' for details
* Add embedded EPMD. Trying to start internal epmd service on starting ergonode.
