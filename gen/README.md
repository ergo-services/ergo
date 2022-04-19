
## Generic behaviors ##

### Server
  Generic server behavior.

### Supervisor
  Generic supervisor behavior.

A supervisor is responsible for starting, stopping, and monitoring its child processes. The basic idea of a supervisor is that it is to keep its child processes alive by restarting them when necessary.

### Application
  Generic application behavior.

### Stage
  Generic stage behavior (originated from Elixir's [GenStage](https://hexdocs.pm/gen_stage/GenStage.html)).

This is abstraction built on top of `gen.Server` to provide a simple way to create a distributed Producer/Consumer architecture, while automatically managing the concept of backpressure. This implementation is fully compatible with Elixir's GenStage. Example is here [examples/genstage](examples/genstage) or just run `go run ./examples/genstage` to see it in action

### Saga
  Generic saga behavior.

It implements Saga design pattern - a sequence of transactions that updates each service state and publishes the result (or cancels the transaction or triggers the next transaction step). `gen.Saga` also provides a feature of interim results (can be used as transaction progress or as a part of pipeline processing), time deadline (to limit transaction lifespan), two-phase commit (to make distributed transaction atomic). Here is example [examples/gensaga](examples/gensaga).

### Raft
  Generic raft behavior.
