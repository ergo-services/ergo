<h1><a href="https://ergo.services"><img src=".github/images/logo.svg" alt="Ergo Framework" width="159" height="49"></a></h1>

[![Gitbook Documentation](https://img.shields.io/badge/GitBook-Documentation-f37f40?style=plastic&logo=gitbook&logoColor=white&style=flat)](https://docs.ergo.services)
[![GoDoc](https://pkg.go.dev/badge/ergo-services/ergo)](https://pkg.go.dev/ergo.services/ergo)
[![MIT license](https://img.shields.io/badge/license-MIT-brightgreen.svg)](https://opensource.org/licenses/MIT)
[![Telegram Community](https://img.shields.io/badge/Telegram-ergo__services-229ed9?style=flat&logo=telegram&logoColor=white)](https://t.me/ergo_services)
[![Twitter](https://img.shields.io/badge/twitter-ergo__services-00acee?style=flat&logo=x&logoColor=white)](https://twitter.com/ergo_services)
[![Reddit](https://img.shields.io/badge/Reddit-r/ergo__services-ff4500?style=plastic&logo=reddit&logoColor=white&style=flat)](https://reddit.com/r/ergo_services)

The Ergo Framework is an implementation of ideas, technologies, and design patterns from the Erlang world in the Go programming language. It is based on the actor model, network transparency, and a set of ready-to-use components for development. This significantly simplifies the creation of complex and distributed solutions while maintaining a high level of reliability and performance.

### Features ###

1. **Actor Model**: enables the creation of scalable and fault-tolerant systems using isolated actors that interact through message passing. Actors can exchange asynchronous messages as well as perform synchronous requests, offering flexibility in communication patterns.

2. **Network Transparency**: actors can interact regardless of their physical location, supported by a high-performance implementation of the [network stack](https://docs.ergo.services/networking/network-stack), which simplifies the creation of distributed systems.

3. **Observability**: framework includes built-in observability features, including [service discovery](https://docs.ergo.services/networking/service-discovering), allowing nodes to automatically register themselves and find routes to remote nodes. This mechanism simplifies managing distributed systems by enabling seamless communication and interaction between nodes across the network.

4. **Ready-to-use Components**: A set of [ready-to-use actors](https://docs.ergo.services/actors) simplifying development, including state management and error handling.

5. **Support for Distributed Systems**: framework includes built-in mechanisms for creating and managing clustered systems, [distributed events](https://docs.ergo.services/basics/events) (publish/subscribe mechanism), [remote actor spawning](https://docs.ergo.services/networking/remote-spawn-process), and [remote application startup](https://docs.ergo.services/networking/remote-start-application). These features enable easy scaling, efficient message broadcasting across your cluster, and the ability to manage distributed components seamlessly.

6. **Reliability and Fault Tolerance**: the framework is designed to minimize failures and ensure automatic recovery, featuring a supervisor tree structure to manage and restart failed actors, which is crucial for mission-critical applications.
   
7. **Flexibility**: This framework offers convenient interfaces for customizing [network stack components](https://docs.ergo.services/networking/network-stack#network-stack-interfaces), creating and integrating custom [loggers](https://docs.ergo.services/basics/logging), [managing SSL certificates](https://docs.ergo.services/basics/certmanager), and more.

In the https://github.com/ergo-services/examples repository, you will find examples that demonstrate a range of the framework's capabilities.

### Observer ###
To inspect the node, network stack, running applications, and processes, you can use the [observer](https://github.com/ergo-services/tools/) tool

<img src="https://github.com/user-attachments/assets/1cb83305-6c56-4eb7-b567-76f3c551c176" width="500">

To install the Observer tool, you need to have the Go compiler version 1.20 or higher. Run the following command:

```
$ go install ergo.services/tools/observer@latest
```

You can also embed the [Observer application](https://docs.ergo.services/application/observer) into your node. To see it in action, see example `demo` at https://github.com/ergo-services/examples. For more information https://docs.ergo.services/tools/observer 



### Quick start ###

For a quick start, use the [ergo](https://docs.ergo.services/tools/ergo) tool — a command-line utility designed to simplify the process of generating boilerplate code for your project based on the Ergo Framework. With this tool, you can rapidly create a complete project structure, including applications, actors, supervisors, network components, and more. It offers a set of arguments that allow you to customize the project according to specific requirements, ensuring it is ready for immediate development.

To install use the following command:

```
$ go install ergo.services/tools/ergo@latest
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

Starting from version 3.0.0, support for the Erlang network stack has been moved to a separate module and is distributed under the BSL 1.1 license - https://github.com/ergo-services/proto. You can find detailed information on using this module in the documentation at https://docs.ergo.services/extra-library/network-protocols/erlang.

### Requirements ###

* Go 1.20.x and above

### Changelog ###

Fully detailed changelog see in the [ChangeLog](ChangeLog.md) file.

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


### Benchmarks ###

You can find available benchmarks in the following repository https://github.com/ergo-services/benchmarks.

* Messaging performance (local, network)

* Memory consumption (demonstrates framework memory footprint) per process.

### Development and debugging ###

To enable Golang profiler just add `--tags debug` in your `go run` or `go build` (profiler runs at
`http://localhost:9009/debug/pprof`)

To disable panic recovery use `--tags norecover`.

To enable trace logging level for the internals (node, network,...) use `--tags trace` and set the log level `gen.LogLevelTrace` for your node.

To run tests with cleaned test cache:

```
go vet
go clean -testcache
go test -v ./tests/...
```

### Commercial support

please, contact support@ergo.services for more information
