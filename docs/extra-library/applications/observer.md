---
description: Real-time web UI for monitoring and inspecting Ergo nodes
---

# Observer

Observer is a web application that embeds into your node and provides real-time visibility into the running system. It uses Server-Sent Events (SSE) for live push updates to the browser.

For a detailed description of the UI and all available views, see [Inspecting With Observer](../../advanced/observer.md).

## Adding Observer to a node

<pre class="language-go"><code class="lang-go">import (
	"ergo.services/ergo"
	"ergo.services/application/observer"
	"ergo.services/ergo/gen"
)

func main() {
	options := gen.NodeOptions{
		Applications: []gen.ApplicationBehavior{
			observer.CreateApp(observer.Options{}),
		},
	}
	node, err := ergo.StartNode("mynode@localhost", options)
	if err != nil {
		panic(err)
	}
	node.Wait()
}
</code></pre>

Open `http://localhost:9911` in your browser.

## Options

`observer.CreateApp` accepts `observer.Options`:

* **Host**: interface to listen on. Default: `localhost`.
* **Port**: HTTP port. Default: `9911`.
* **PoolSize**: number of worker processes handling requests. Default: `10`.
* **LogLevel**: log level for Observer's own processes. Default: `gen.LogLevelInfo`.

## What Observer shows

### Node

General node information: name, version, OS, architecture, CPU cores, timezone, uptime, memory usage, process count, goroutine count. Memory graph updates live over the last 60 seconds. Node-level log level can be changed directly from this view.

### Processes

Full process list with per-process metrics: state, mailbox depth, message latency, running time, wakeup count, uptime. Supports filtering by name pattern, behavior type, application, state, and minimum mailbox depth.

Clicking a process opens its detail view: supervision tree position, links, monitors, registered names, aliases, environment variables, and the internal state returned by `HandleInspect`.

Any actor that implements `HandleInspect` exposes its state as a live-updating key-value panel in the browser:

```go
func (a *MyActor) HandleInspect(from gen.PID, item ...string) map[string]string {
    return map[string]string{
        "connections": fmt.Sprintf("%d", a.connCount),
        "last_error":  a.lastError,
    }
}
```

### Meta-processes

Meta-processes (TCP servers, WebSocket handlers, SSE handlers, Port processes, and others) with their state, type, and parent process.

### Applications

All loaded applications with state (loaded, running, stopping), mode, uptime, and their process groups. Full application process tree on click. Applications can be started, stopped, and unloaded from this view.

### Network

Network stack details: mode, acceptors, protocol and handshake versions, registrar. Below the top stat cards and acceptors, three tabs are available:

* **Connections** lists all active remote node connections with traffic counters and sortable columns.
* **Routes** shows configured static routes and proxy routes.
* **Types** shows the wire-format type registry (one entry per proto): registration ID, name, kind, and the inferred schema. Two filters narrow the list by name and by schema content. The data is captured on demand via the Refresh button.

### Events

All registered events: producer PID, subscriber count, buffered flag, and publication statistics. Filter by name, notification mode, buffered mode, and minimum subscriber count.

### Logs

Live log stream from the observed node. Filter by level (debug, info, warning, error, panic).

### Profiler

**Goroutines** - dump of all goroutines with stack traces, grouping, and filtering by state and minimum wait time.

**Heap** - allocation profile showing top call sites by bytes. Filter by minimum allocation size.

Both are available without restarting the node or enabling any special build flags.

## Actions

Observer is not read-only. From process and meta-process views you can:

* **Send a message** to a process or meta-process
* **Send an exit signal** with a custom reason
* **Kill** a process
* **Change log level** for the node, a specific process, or a specific meta-process
* **Adjust per-process network settings**: send priority, message ordering (`KeepNetworkOrder`), important delivery, compression type/level/threshold

## Inspecting the whole cluster

Observer communicates with the `system` application, which is started automatically on every Ergo node. Because of this, a single Observer instance can switch to any node in the cluster and inspect it without deploying anything extra to that node. Use the node selector in the UI to connect to any cluster node, via the registrar if configured, or by entering the host, port, and cookie explicitly.
