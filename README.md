<h1><a href="https://ergo.services"><img src=".github/images/logo.svg" alt="Ergo Framework" width="159" height="49"></a></h1>

[![Gitbook Documentation](https://img.shields.io/badge/GitBook-Documentation-f37f40?style=plastic&logo=gitbook&logoColor=white&style=flat)](https://docs.ergo.services)
[![GoDoc](https://pkg.go.dev/badge/ergo-services/ergo)](https://pkg.go.dev/ergo.services/ergo)
[![MIT license](https://img.shields.io/badge/license-MIT-brightgreen.svg)](https://opensource.org/licenses/MIT)
[![Telegram Community](https://img.shields.io/badge/Telegram-ergo__services-229ed9?style=flat&logo=telegram&logoColor=white)](https://t.me/ergo_services)
[![Twitter](https://img.shields.io/badge/twitter-ergo__services-00acee?style=flat&logo=x&logoColor=white)](https://x.com/ergo_services)
[![Reddit](https://img.shields.io/badge/Reddit-r/ergo__services-ff4500?style=plastic&logo=reddit&logoColor=white&style=flat)](https://reddit.com/r/ergo_services)

The Ergo Framework is an implementation of ideas, technologies, and design patterns from the Erlang world in the Go programming language. It is based on the actor model, network transparency, and a set of ready-to-use components for development. This significantly simplifies the creation of complex and distributed solutions while maintaining a high level of reliability and performance.

### Features ###

1. **Actor Model**: enables the creation of scalable and fault-tolerant systems using isolated actors that interact through message passing. Actors can exchange asynchronous messages as well as perform synchronous requests, offering flexibility in communication patterns.

2. **Network Transparency**: actors can interact regardless of their physical location, supported by a [high-performance](https://github.com/ergo-services/benchmarks) implementation of the [network stack](https://docs.ergo.services/networking/network-stack), which simplifies the creation of distributed systems.

3. **Observability**: framework includes built-in observability features, including [service discovery](https://docs.ergo.services/networking/service-discovering) and [static routes](https://docs.ergo.services/networking/static-routes), allowing nodes to automatically register themselves and find routes to remote nodes. This mechanism simplifies managing distributed systems by enabling seamless communication and interaction between nodes across the network.

4. **Ready-to-use Components**: A set of [ready-to-use actors](https://docs.ergo.services/actors) simplifying development, including state management and error handling.

5. **Support for Distributed Systems**: framework includes built-in mechanisms for creating and managing clustered systems, [distributed events](https://docs.ergo.services/basics/events) (publish/subscribe mechanism), [remote actor spawning](https://docs.ergo.services/networking/remote-spawn-process), and [remote application startup](https://docs.ergo.services/networking/remote-start-application). These features enable easy scaling, efficient message broadcasting across your cluster, and the ability to manage distributed components seamlessly.

6. **Reliability and Fault Tolerance**: the framework is designed to minimize failures and ensure automatic recovery, featuring a [supervisor tree](https://docs.ergo.services/basics/supervision-tree) structure to manage and [restart failed actors](https://docs.ergo.services/actors/supervisor#restart-strategy), which is crucial for mission-critical applications.
   
7. **Flexibility**: This framework offers convenient interfaces for customizing [network stack components](https://docs.ergo.services/networking/network-stack#network-stack-interfaces), creating and integrating custom [loggers](https://docs.ergo.services/basics/logging), [managing SSL certificates](https://docs.ergo.services/basics/certmanager), and more.

In the https://github.com/ergo-services/examples repository, you will find examples that demonstrate a range of the framework's capabilities.

### Benchmarks ###

On a 64-core processor, Ergo Framework demonstrates a performance of **over 21 million messages per second locally** and **nearly 5 million messages per second over the network**.

![image](https://github.com/user-attachments/assets/813900ad-ff79-4cc7-b1e6-396c5781acb1)

You can find available benchmarks in the following repository https://github.com/ergo-services/benchmarks.

* Messaging performance (local, network)

* Memory consumption per process (demonstrates framework memory footprint).

* Serialization performance comparison: EDF vs Protobuf vs Gob

### Observer ###
To inspect the node, network stack, running applications, and processes, you can use the [`observer`](https://github.com/ergo-services/tools/) tool

<img src="https://github.com/user-attachments/assets/1cb83305-6c56-4eb7-b567-76f3c551c176" width="500">

To install the Observer tool, you need to have the Go compiler version 1.20 or higher. Run the following command:

```
$ go install ergo.tools/observer@latest
```

You can also embed the [Observer application](https://docs.ergo.services/extra-library/applications/observer) into your node. To see it in action, see example `demo` at https://github.com/ergo-services/examples. For more information https://docs.ergo.services/tools/observer 



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

Starting from version 3.0.0, support for the Erlang network stack has been moved to a separate module - https://github.com/ergo-services/proto. Version 3.0 was distributed under the BSL 1.1 license, but starting from version 3.1 it is available under the MIT license. You can find detailed information on using this module in the documentation at https://docs.ergo.services/extra-library/network-protocols/erlang.

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
* Completely reworked internal **Target Manager** (`node/tm/`) - improved architecture for process, event, and node target management with comprehensive test coverage
* Completely reworked internal **Pub/Sub** mechanism - improved reliability and performance
* Improved **ProcessInit state** - more `gen.Process` methods now available during initialization:
  - `Link*`, `Unlink*`, `Monitor*`, `Demonitor*`
  - `Call*`, `Inspect`, `InspectMeta`
  - `RegisterName`, `UnregisterName`, `RegisterEvent`, `UnregisterEvent`
  - `CreateAlias`, `DeleteAlias`
  - `SendEvent`, `Info`, `MetaInfo`
* Improved API documentation - comprehensive godoc comments for all public interfaces
* **Documentation rewritten** - complete documentation now included in the repository (`docs/`) and available at https://docs.ergo.services
* New documentation section **Advanced**:
  - [Handle Sync](https://docs.ergo.services/advanced/handle-sync) - synchronous message handling patterns
  - [Important Delivery](https://docs.ergo.services/advanced/important-delivery) - guaranteed delivery mechanism
  - [Pub/Sub Internals](https://docs.ergo.services/advanced/pub-sub-internals) - event system architecture

* **Extra Library - Actors** (https://github.com/ergo-services/actor):
  - Introduced **Leader** actor - distributed leader election with Raft-inspired consensus algorithm. Features: term-based disambiguation, automatic failover, split-brain prevention through majority quorum, dynamic peer discovery. See [documentation](https://docs.ergo.services/extra-library/actors/leader)
  - Introduced **Metrics** actor - Prometheus metrics exporter that collects node/network telemetry via HTTP endpoint. Features: automatic collection of node metrics (uptime, processes, memory), network metrics per remote node, extensible for custom metrics. See [documentation](https://docs.ergo.services/extra-library/actors/metrics)

* **Benchmarks** (https://github.com/ergo-services/benchmarks):
  - Introduced **Distributed Pub/Sub** benchmark - demonstrates event delivery to 1,000,000 subscribers across 10 nodes. Achieves 2.9M msg/sec delivery rate with only 10 network messages (one per consumer node) instead of 1M


### Development and debugging ###

To enable Golang profiler just add `--tags debug` in your `go run` or `go build` (profiler runs at
`http://localhost:9009/debug/pprof`)

To disable panic recovery use `--tags norecover`.

To enable trace logging level for the internals (node, network,...) use `--tags trace` and set the log level `gen.LogLevelTrace` for your node.

To run tests with cleaned test cache:

```
go vet
go clean -testcache
go test -v ./testing/tests/...
```

### Commercial support

please, contact support@ergo.services for more information
