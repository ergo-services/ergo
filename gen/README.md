
## Generic behaviors ##

### Server
  Generic server behavior.

Example: [gen.Server](https://github.com/ergo-services/examples/tree/master/genserver)

### Supervisor
  Generic supervisor behavior.

A supervisor is responsible for starting, stopping, and monitoring its child processes. The basic idea of a supervisor is that it is to keep its child processes alive by restarting them when necessary.

Example: [gen.Supervisor](https://github.com/ergo-services/examples/tree/master/supervisor)

### Application
  Generic application behavior.

Example: [gen.Application](https://github.com/ergo-services/examples/tree/master/application)

### Pool
  Generic pool of workers.

  This behavior implements a basic design pattern with a pool of workers. All messages/requests received by the pool process are forwarded to the workers using the "Round Robin" algorithm. The worker process is automatically restarting on termination.

Example: [gen.Pool](https://github.com/ergo-services/examples/tree/master/genpool)

### Web
  Web API Gateway behavior.

  The Web API Gateway pattern is also sometimes known as the "Backend For Frontend" (BFF)  because you build it while thinking about the needs of the client app. Therefore, BFF sits between the client apps and the microservices. It acts as a reverse proxy, routing requests from clients to services.

Example: [gen.Web](https://github.com/ergo-services/examples/tree/master/genweb)

### TCP
  Socket acceptor pool for TCP protocols.

  This behavior aims to provide everything you need to accept TCP connections and process packets with a small code base and low latency while being easy to use.

Example: [gen.TCP](https://github.com/ergo-services/examples/tree/master/gentcp)

### UDP
  UDP acceptor pool for UDP protocols

  This behavior provides the same feature set as TCP but for handling UDP packets using pool of handlers.

Example: [gen.UDP](https://github.com/ergo-services/examples/tree/master/genudp)

### Stage
  Generic stage behavior (originated from Elixir's [GenStage](https://hexdocs.pm/gen_stage/GenStage.html)).

This is abstraction built on top of `gen.Server` to provide a simple way to create a distributed Producer/Consumer architecture, while automatically managing the concept of backpressure. This implementation is fully compatible with Elixir's GenStage.

Example: [gen.Stage](https://github.com/ergo-services/examples/tree/master/genstage)

### Saga
  Generic saga behavior.

It implements Saga design pattern - a sequence of transactions that updates each service state and publishes the result (or cancels the transaction or triggers the next transaction step). `gen.Saga` also provides a feature of interim results (can be used as transaction progress or as a part of pipeline processing), time deadline (to limit transaction lifespan), two-phase commit (to make distributed transaction atomic).

Example: [gen.Saga](https://github.com/ergo-services/examples/tree/master/gensaga)

### Raft
  Generic raft behavior.

It's improved implementation of [Raft consensus algorithm](https://raft.github.io). The key improvement is using quorum under the hood to manage the leader election process and make the Raft cluster more reliable. This implementation supports quorums of 3, 5, 7, 9, or 11 quorum members.

Example: [gen.Raft](https://github.com/ergo-services/examples/tree/master/genraft)
