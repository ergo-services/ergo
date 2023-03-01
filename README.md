<h1><a href="https://ergo.services"><img src=".github/images/logo.svg" alt="Ergo Framework" width="159" height="49"></a></h1>

<!--[![Gitbook Documentation](https://img.shields.io/badge/GitBook-Documentation-f37f40?style=plastic&logo=gitbook&logoColor=white&style=flat)](https://docs.ergo.services) -->
[![GoDoc](https://pkg.go.dev/badge/ergo-services/ergo)](https://pkg.go.dev/github.com/ergo-services/ergo)
[![MIT license](https://img.shields.io/badge/license-MIT-brightgreen.svg)](https://opensource.org/licenses/MIT)
[![Telegram Community](https://img.shields.io/badge/Telegram-Community-blue?style=flat&logo=telegram)](https://t.me/ergo_services)
[![Discord Community](https://img.shields.io/badge/Discord-Community-5865F2?style=flat&logo=discord&logoColor=white)](https://discord.gg/sdscxKGV62)
[![Twitter](https://img.shields.io/badge/Twitter-ergo__services-1DA1F2?style=flat&logo=twitter&logoColor=white)](https://twitter.com/ergo_services)

Technologies and design patterns of Erlang/OTP have been proven over the years. Now in Golang.
Up to x5 times faster than original Erlang/OTP in terms of network messaging.
The easiest way to create an OTP-designed application in Golang.

[https://ergo.services](https://ergo.services)

### Purpose ###

The goal of this project is to leverage Erlang/OTP experience with Golang performance. Ergo Framework implements [DIST protocol](https://erlang.org/doc/apps/erts/erl_dist_protocol.html), [ETF data format](https://erlang.org/doc/apps/erts/erl_ext_dist.html) and [OTP design patterns](https://erlang.org/doc/design_principles/des_princ.html) `gen.Server`, `gen.Supervisor`, `gen.Application` which makes you able to create distributed, high performance and reliable microservice solutions having native integration with Erlang infrastructure

### Cloud ###

Distributed Cloud is coming. With Ergo Framework you can join your services into a single cluster with transparent networking using our **Cloud Overlay Network** where they can connect to each other smoothly, no matter where they run - AWS, Azure or GCP, or anywhere else. All these connections are secured with end-to-end encryption. Read more in this article [https://blog.ergo.services/cloud-overlay-network-3a133d47efe5](https://blog.ergo.services/cloud-overlay-network-3a133d47efe5).

### Features ###

![image](https://user-images.githubusercontent.com/118860/113710255-c57d5500-96e3-11eb-9970-20f49008a990.png)

* Support Erlang 25 - allows you connect your node to (and accept connection from) any Erlang/Elixir node within a cluster
* Spawn Erlang-like processes
* Register/unregister processes with simple atom
* Set of ready-to-use disign patterns (behaviors)
  * `gen.Server` behavior with atomic state and Erlang's gen_server support to make sync request `ServerProcess.Call`, async - `ServerProcess.Cast` or `Process.Send` in fashion of `gen_server:call`, `gen_server:cast`, `erlang:send` accordingly
  * `gen.Supervisor` behavior with all known [restart strategies](https://erlang.org/doc/design_principles/sup_princ.html#restart-strategy) (One For One, One For All, Rest For One, Simple One For One)
  * `gen.Application` behavior with all known [starting types](https://erlang.org/doc/design_principles/applications.html#application-start-types) (Permanent, Temporary, Transient)
  * `gen.Pool` a basic design pattern with a pool of workers. All messages/requests received by the pool process are forwarded to the workers using the "Round Robin" algorithm. The worker process is automatically restarting on termination
  * `gen.TCP` - socket acceptor pool for TCP protocols. This behavior aims to provide everything you need to accept TCP connections and process packets with a small code base and low latency while being easy to use.
  * `gen.UDP` - acceptor pool for UDP protocols. This behavior provides the same feature set as TCP but for handling UDP packets using pool of handlers
  * `gen.Web` - Web API Gateway behavior. This behavior allows you to listen HTTP port and handle HTTP-request using pool of workers.
  * `gen.Stage` behavior support (originated from Elixir's [GenStage](https://hexdocs.pm/gen_stage/GenStage.html)). This is abstraction built on top of `gen.Server` to provide a simple way to create a distributed Producer/Consumer architecture, while automatically managing the concept of backpressure. This implementation is fully compatible with Elixir's GenStage. Example is here [examples/genstage](https://github.com/ergo-services/examples/tree/master/genstage) or just run `go run ./examples/genstage` to see it in action
  * `gen.Saga` behavior support. It implements Saga design pattern - a sequence of transactions that updates each service state and publishes the result (or cancels the transaction or triggers the next transaction step). `gen.Saga` also provides a feature of interim results (can be used as transaction progress or as a part of pipeline processing), time deadline (to limit transaction lifespan), two-phase commit (to make distributed transaction atomic). Here is example [examples/gensaga](https://github.com/ergo-services/examples/tree/master/gensaga).
  * `gen.Raft` behavior support. It's improved implementation of [Raft consensus algorithm](https://raft.github.io). The key improvement is using quorum under the hood to manage the leader election process and make the Raft cluster more reliable. This implementation supports quorums of 3, 5, 7, 9, or 11 quorum members. Here is an example of this feature [examples/genraft](https://github.com/ergo-services/examples/tree/master/genraft)
* Monitor processes/nodes, local/remote with Erlang support
* Link processes local/remote with Erlang support
* [embedded EPMD](#epmd) (in order to get rid of erlang' dependencies) with Erlang support
* Unmarshalling terms into the struct using `etf.TermIntoStruct`, `etf.TermProplistIntoStruct` or to the string using `etf.TermToString` including custom marshaling/unmarshaling via `Marshal` and `Unmarshal` interfaces. But it's highly recommended to use `etf.RegisterType` so you will be receiving messages in a native Golang-type
* Encryption (TLS 1.3) support (including autogenerating self-signed certificates)
* Compression support (with customization of compression level and threshold). It can be configured for the node or a particular process.
* Proxy support with end-to-end encryption, includeing compression/fragmentation/linking/monitoring features.
* Tested and confirmed support Windows, Darwin (MacOS), Linux, FreeBSD.
* Zero dependencies. All features are implemented using the standard Golang library.

### Requirements ###

* Go 1.17.x and above

### Versioning ###

Golang introduced [v2 rule](https://go.dev/blog/v2-go-modules) a while ago to solve complicated dependency issues. We found this solution very controversial and there is still a lot of discussion around it. So, we decided to keep the old way for the versioning, but have to use the git tag with v1 as a major version (due to "v2 rule" restrictions). Since now we use git tag pattern 1.999.XYZ where X - major number, Y - minor, Z - patch version.

### Changelog ###

Here are the changes of latest release. For more details see the [ChangeLog](ChangeLog.md)

#### [v2.2.2](https://github.com/ergo-services/ergo/releases/tag/v1.999.222) 2023-03-01 [tag version v1.999.222] ####

* Introduced `gen.Pool`. This behavior implements a basic design pattern with a pool of workers. All messages/requests received by the pool process are forwarded to the workers using the "Round Robin" algorithm. The worker process is automatically restarting on termination. See example here [examples/genpool](https://github.com/ergo-services/examples/tree/master/genpool)
* Removed Erlang RPC support. A while ago Erlang has changed the way of handling this kind of request making this feature more similar to the regular `gen.Server`. So, there is no reason to keep supporting it. Use a regular way of messaging instead - `gen.Server`. 
* Fixed issue #130 (`StartType` option in `gen.ApplicationSpec` is ignored for the autostarting applications)
* Fixed issue #143 (incorrect cleaning up the aliases belonging to the terminated process)

#### [v2.2.1](https://github.com/ergo-services/ergo/releases/tag/v1.999.221) 2023-02-01 [tag version v1.999.221] ####

* Now you can join your services made with Ergo Framework into a single cluster with transparent networking using our **Cloud Overlay Network** where they can connect to each other smoothly, no matter where they run - AWS, Azure or GCP, or anywhere else. All these connections are secured with end-to-end encryption. Read more in this article [https://blog.ergo.services/cloud-overlay-network-3a133d47efe5](https://blog.ergo.services/cloud-overlay-network-3a133d47efe5). Here is an example of this feature in action [examples/cloud](https://github.com/ergo-services/examples/tree/master/cloud)
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

### Benchmarks ###

Here is simple EndToEnd test demonstrates performance of messaging subsystem

Hardware: workstation with AMD Ryzen Threadripper 3970X (64) @ 3.700GHz

```
❯❯❯❯ go test -bench=NodeParallel -run=XXX -benchtime=10s
goos: linux
goarch: amd64
pkg: github.com/ergo-services/ergo/tests
cpu: AMD Ryzen Threadripper 3970X 32-Core Processor
BenchmarkNodeParallel-64                 4738918              2532 ns/op
BenchmarkNodeParallelSingleNode-64      100000000              429.8 ns/op

PASS
ok      github.com/ergo-services/ergo/tests  29.596s
```

these numbers show almost **500.000 sync requests per second** for the network messaging via localhost and **10.000.000 sync requests per second** for the local messaging (within a node).

#### Compression

This benchmark shows the performance of compression for sending 1MB message between two nodes (via a network).

```
❯❯❯❯ go test -bench=NodeCompression -run=XXX -benchtime=10s
goos: linux
goarch: amd64
pkg: github.com/ergo-services/ergo/tests
cpu: AMD Ryzen Threadripper 3970X 32-Core Processor
BenchmarkNodeCompressionDisabled1MBempty-64         2400           4957483 ns/op
BenchmarkNodeCompressionEnabled1MBempty-64          5769           2088051 ns/op
BenchmarkNodeCompressionEnabled1MBstring-64         5202           2077099 ns/op
PASS
ok      github.com/ergo-services/ergo/tests     56.708s
```

It demonstrates **more than 2 times** improvement.

#### Proxy

This benchmark demonstrates how proxy feature and e2e encryption impact a messaging performance.

```
❯❯❯❯ go test -bench=NodeProxy -run=XXX -benchtime=10s
goos: linux
goarch: amd64
pkg: github.com/ergo-services/ergo/tests
cpu: AMD Ryzen Threadripper 3970X 32-Core Processor
BenchmarkNodeProxy_NodeA_to_NodeC_direct_Message_1KB-64                     1908477       6337 ns/op
BenchmarkNodeProxy_NodeA_to_NodeC_via_NodeB_Message_1KB-64                  1700984       7062 ns/op
BenchmarkNodeProxy_NodeA_to_NodeC_via_NodeB_Message_1KB_Encrypted-64        1271125       9410 ns/op
PASS
ok      github.com/ergo-services/ergo/tests     45.649s

```


#### Ergo Framework vs original Erlang/OTP

Hardware: laptop with Intel(R) Core(TM) i5-8265U (4 cores. 8 with HT)

![benchmarks](https://raw.githubusercontent.com/halturin/ergobenchmarks/master/ergobenchmark.png)

sources of these benchmarks are [here](https://github.com/halturin/ergobenchmarks)


### EPMD ###

*Ergo Framework* has embedded EPMD implementation in order to run your node without external epmd process needs. By default, it works as a client with erlang' epmd daemon or others ergo's nodes either.

The one thing that makes embedded EPMD different is the behavior of handling connection hangs - if ergo' node is running as an EPMD client and lost connection, it tries either to run its own embedded EPMD service or to restore the lost connection.

### Examples ###

Code below is a simple implementation of gen.Server pattern [examples/genserver](https://github.com/ergo-services/examples/tree/master/genserver)

```golang
package main

import (
	"fmt"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

type simple struct {
	gen.Server
}

func (s *simple) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	value := message.(int)
	fmt.Printf("HandleInfo: %#v \n", message)
	if value > 104 {
		return gen.ServerStatusStop
	}
	// sending message with delay 1 second
	fmt.Println("increase this value by 1 and send it to itself again")
	process.SendAfter(process.Self(), value+1, time.Second)
	return gen.ServerStatusOK
}

```

here is output of this code

```shell
$ go run ./examples/simple
HandleInfo: 100
HandleInfo: 101
HandleInfo: 102
HandleInfo: 103
HandleInfo: 104
HandleInfo: 105
exited
```

See [https://github.com/ergo-services/examples](https://github.com/ergo-services/examples/) for more details

* [gen.Application](https://github.com/ergo-services/examples/tree/master/application)
* [gen.Supervisor](https://github.com/ergo-services/examples/tree/master/supervisor)
* [gen.Server](https://github.com/ergo-services/examples/tree/master/genserver)
* [gen.Pool](https://github.com/ergo-services/examples/tree/master/genpool)
* [gen.Stage](https://github.com/ergo-services/examples/tree/master/genstage)
* [gen.Saga](https://github.com/ergo-services/examples/tree/master/gensaga)
* [gen.Raft](https://github.com/ergo-services/examples/tree/master/genraft)
* [gen.Custom](https://github.com/ergo-services/examples/tree/master/gencustom)
* [gen.Web](https://github.com/ergo-services/examples/tree/master/genweb)
* [gen.TCP](https://github.com/ergo-services/examples/tree/master/gentcp)
* [gen.UDP](https://github.com/ergo-services/examples/tree/master/genudp)
* [events](https://github.com/ergo-services/examples/tree/master/events)
* [erlang](https://github.com/ergo-services/examples/tree/master/erlang)
* [proxy](https://github.com/ergo-services/examples/tree/master/proxy)
* [cloud](https://github.com/ergo-services/examples/tree/master/cloud)

### Elixir Phoenix Users ###

Users of the Elixir Phoenix framework might encounter timeouts when trying to connect a Phoenix node
to an ergo node. The reason is that, in addition to global_name_server and net_kernel,
Phoenix attempts to broadcast messages to the [pg2 PubSub handler](https://hexdocs.pm/phoenix/1.1.0/Phoenix.PubSub.PG2.html)

To work with Phoenix nodes, you must create and register a dedicated pg2 GenServer, and
spawn it inside your node. The spawning process must have "pg2" as a process name:

```golang
type Pg2GenServer struct {
    gen.Server
}

func main() {
    // ...
    pg2 := &Pg2GenServer{}
    node1, _ := ergo.StartNode("node1@localhost", "cookies", node.Options{})
    process, _ := node1.Spawn("pg2", gen.ProcessOptions{}, pg2, nil)
    // ...
}
```

### Development and debugging ###

There are options already defined that you might want to use

* `-ergo.trace` - enable debug info (logging via `lib.Log(...)`)
* `-ergo.debug` - enable extended debug info (logging via `lib.Log(...)` and `lib.Warning(...)`)
* `-ergo.norecover` - disable panic catching
* `-ergo.warning` - enable/disable warnings (logging via `lib.Warning(...)`. Default: enable)

To enable Golang profiler just add `--tags debug` in your `go run` or `go build` like this:

```
go run --tags debug ./examples/genserver/demoGenServer.go
```

Now golang' profiler is available at `http://localhost:9009/debug/pprof`

To check test coverage:

```
go test -coverprofile=cover.out ./...
go tool cover -html=cover.out -o coverage.html
```

To run tests with cleaned test cache:

```
go vet
go clean -testcache
go test -v ./...
```

To run benchmarks:

```
go test -bench=Node -run=X -benchmem
```

### Companies are using Ergo Framework ###

[![Kaspersky](.github/images/kaspersky.png)](https://kaspersky.com)
[![RingCentral](.github/images/ringcentral.png)](https://www.ringcentral.com)
[![LilithGames](.github/images/lilithgames.png)](https://lilithgames.com)

is your company using Ergo? add your company logo/name here

### Commercial support

please, contact ceo@ergo.services for more information
