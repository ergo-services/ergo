---
description: '"Pool of Workers" design pattern'
---

# Pool

The `act.Pool` actor implements the low-level `gen.ProcessBehavior` interface and provides the "_Pool of Workers_" design pattern functionality. All asynchronous messages and synchronous requests sent to the _pool_ process are redirected to _worker_ processes in the _pool_.

To launch a process based on `act.Pool`, you need to embed it in your object. For example:

<pre class="language-go" data-full-width="false"><code class="lang-go">func factory_myPool() gen.ProcessBehavior {
    return &#x26;myPool{}
}

<strong>type myPool struct {
</strong>    act.Pool
}
...
node.Spawn(factory_myPool, gen.ProcessOptions)
</code></pre>

`act.Pool` uses the `act.PoolBehavior` interface to interact with your object:

```go
type PoolBehavior interface {
	gen.ProcessBehavior
	
	Init(args ...any) (PoolOptions, error)

	HandleMessage(from gen.PID, message any) error
	HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)
	Terminate(reason error)

	HandleInspect(from gen.PID) map[string]string
	HandleEvent(message gen.MessageEvent) error
}

```

The `Init` method is mandatory for implementation in `act.PoolBehavior`, while the other methods are optional. Similar to `act.Actor`, `act.Pool` has the embedded `gen.Process` interface, allowing you to use its methods directly from your object:

```go
func (p *myPool) Init(args...) (act.PoolOptions, error) {
    var options act.PoolOptions
    p.Log().Info("starting pool process with name", p.Name())
    // ...
    // set pool options
    // ...
    return options, nil
}

func (p *myPool) Terminate(reason error) {
    p.Log().Info("pool process terminated with reason: %s", reason)
}
```

### act.PoolOptions

With `act.PoolOptions`, you can configure various parameters for the _pool_, such as:

```go
type PoolOptions struct {
	PoolSize          int64
	WorkerFactory     gen.ProcessFactory
	WorkerMailboxSize int64
	WorkerArgs        []any
}
```

* **PoolSize**: Determines the number of _worker_ processes to be launched when your _pool_ process starts. If this option is not set, the default value of 3 is used.
* **WorkerFactory**: A factory function that creates _worker_ processes.
* **WorkerMailboxSize**: Specifies the size of the mailbox for each _worker_ process.
* **WorkerArgs**: Arguments passed during the startup of _worker_ processes.

### Load distribution

All asynchronous messages and synchronous requests received in the _pool_ process's mailbox are automatically forwarded to _worker_ processes, distributing the load evenly. The load is distributed using a FIFO queue of _worker_ processes.

**Message/Request Handling Algorithm:**

1. Select a _worker_ from the FIFO queue.
2. Forward the message/request.
3. If errors like `gen.ErrProcessUnknown` or `gen.ErrProcessTerminated` occur, a new _worker_ is started, and the message is sent to it.
4. If `gen.ErrMailboxFull` occurs, the _worker_ is requeued, and the next _worker_ is selected.
5. Upon successful forwarding, the _worker_ returns to the end of the queue.

If all _workers_ are busy (e.g., due to `gen.ErrMailboxFull`), the _pool_ process logs an error, such as `"no available worker process. ignored message from <PID>."`

Forwarding to _workers_ only happens for messages from the _Main_ queue. To ensure a message is handled by the _pool_ process itself, send it with a priority higher than `gen.MessagePriorityNormal`. The `gen.Process` interface provides `SendWithPriority` and `CallWithPriority` methods for sending messages or making synchronous requests with a specified priority.

### Methods of act.Pool

To dynamically manage the number of _worker_ processes, `act.Pool` provides the following methods:

* **AddWorkers**: This method adds a specified number of _worker_ processes to the _pool_. Upon successful execution, it returns the total number of _worker_ processes in the _pool_ after the addition.
* **RemoveWorkers**: This method stops and removes a specified number of _worker_ processes from the _pool_. Upon successful execution, it returns the total number of _worker_ processes remaining in the _pool_ after the removal.

