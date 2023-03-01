# Changelog
All notable changes to this project will be documented in this file.

This format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
