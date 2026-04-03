---
description: Real-time inspection and management of Ergo nodes
---

# Inspecting With Observer

Running an actor system is straightforward until something unexpected happens. A process stops responding. Memory usage grows. A message chain takes longer than expected. At that point, you need to see inside the running system without stopping it.

Observer is a web dashboard that runs as an application on your Ergo node. It gives you a live view of everything happening on the node and lets you change configuration, send messages, and control application lifecycle without restarting anything. You interact with it through a browser while the node continues running.

## Adding to Your Node

```go
import (
    "ergo.services/application/observer"
    "ergo.services/ergo"
    "ergo.services/ergo/gen"
)

func main() {
    node, err := ergo.StartNode("mynode@localhost", gen.NodeOptions{
        Applications: []gen.ApplicationBehavior{
            observer.CreateApp(observer.Options{
                Port: 9911,
            }),
        },
    })
    if err != nil {
        panic(err)
    }
    node.Wait()
}
```

Open `http://localhost:9911` in a browser. You land on the dashboard showing the node where Observer is running. Change `Host` to `"0.0.0.0"` if you need access from another machine.

Observer can inspect any node in the cluster, not just the one it runs on. The sidebar contains a node selector listing all nodes discovered through the registrar. Select a different node and Observer switches to showing that node's data. The remote node needs the `system_inspect` process, which is included by default in every Ergo node. This means you deploy Observer on one node and monitor the entire cluster from a single browser tab.

When switching to a remote node, you can provide connection parameters (cookie, host, port, TLS) if the node is not yet connected. If the nodes are already connected through the registrar, switching is instant.

## Dashboard

The dashboard is the landing page. It answers the first question you ask about any running system: is everything normal?

<figure><img src="../.gitbook/assets/observer.png" alt="Observer Dashboard"><figcaption></figcaption></figure>

Three summary cards at the top give you the pulse of the node. The Processes card shows the total count, how many are actively running, and whether any are stuck as zombies. The Events card shows message throughput across the pub/sub system. The System card shows memory and CPU usage, core count, and uptime. Each card includes a real-time chart covering the last 60 seconds. A sudden spike in running processes, a memory growth pattern, or unusual CPU load is immediately visible.

Below the summary, detailed panels break down the node's internal counters. The processes panel shows spawned, terminated, and spawn-failed counts. Spawn failures are highlighted so they catch your attention. The registry panel shows how many names, aliases, and events are registered. The delivery errors panel highlights any failed message deliveries, both local and remote. Non-zero values are shown in red. If you see errors here, messages are being lost somewhere.

Two controls are available directly on the dashboard. The log level dropdown changes the node-level log severity threshold. The tracing sampler dropdown controls whether the node starts new traces for messages sent via `node.Send()` and `node.Call()`. Both take effect immediately without restarting anything.

The loggers panel shows a distribution of log messages by severity and lists all registered loggers with their level filters. This tells you at a glance whether errors are accumulating faster than expected. The tracing exporters panel shows the same for tracing: span distribution by kind and the list of active exporters with their flags. If you expected Pulse to be exporting traces but don't see it listed here, you know something went wrong during startup.

A searchable table of cron jobs at the bottom shows scheduled tasks with their names, cron specs, descriptions, and last run times. You can filter by name or spec pattern to find specific jobs.

## Processes

The processes page is where you spend most of your time when investigating issues.

Every process on the node appears in a table that updates every second. You see the PID, registered name, behavior type, application, messages in and out, mailbox depth, processing latency, running time, initialization time, wakeup count, uptime, and current state. This is enough to answer most diagnostic questions without opening individual process details.

When message counts change between updates, a green delta indicator appears next to the number. A "+42" next to Messages In tells you this process received 42 messages in the last second. The mailbox column changes color as the queue grows, making overloaded processes visually obvious in a list of thousands. The state column shows how long the process has been in its current state. A process stuck in "running" for 30 seconds is probably blocked inside a handler.

All columns are sortable. Clicking Messages In sorts by busiest processes. Clicking Mailbox puts the most backlogged processes at the top. Clicking Running Time reveals which processes spend the most time executing handlers.

With thousands of processes, you need to narrow the view. The Scope panel controls what the server sends to the browser. This is server-side filtering: the node only transmits matching processes, keeping the data flow manageable. The Show control sets the count (presets from 100 to 1000). The From control chooses the starting point: First (oldest PIDs), Last (newest), or a specific PID. Filters narrow by name, behavior, application, state, or minimum mailbox depth. Active filters appear as removable chips in the toolbar. A separate search field adds client-side regex filtering on top for quick ad-hoc lookups.

Click any PID to open a floating detail window.

<details>
<summary>Processes page with scope panel</summary>

<!-- screenshot: processes page with scope panel open -->

</details>

## Process Details

Floating detail windows are the primary tool for investigating individual processes. Multiple windows can be open simultaneously. They persist when you switch between pages, so you can keep a problematic process open while you check logs or traces elsewhere.

The overview tab shows two real-time charts. The messages chart tracks incoming and outgoing message rates over the last 60 seconds, with a toggle between rate and cumulative views. The mailbox chart tracks the four queue depths: Main, System, Urgent, and Log. Below the charts, cards show running time, init time, and uptime. If the init time is suspiciously long, you know the process took a while to start. If the running time is high relative to uptime, the process is spending most of its life inside handlers rather than waiting for messages. The parent and leader processes appear as clickable links that open their own windows.

The relations tab reveals the process's connections: aliases it has registered, meta processes it owns, events it has created, and its links and monitors grouped by type. This is valuable when you need to understand the supervision tree or figure out which processes will be affected if this one terminates.

The config tab lets you change settings that take effect immediately. You can raise the log level to get more verbose output from a specific process, enable compression for network messages, change the tracing sampler for targeted diagnostics, or adjust message priority and delivery guarantees. The environment variables section is available if the node has `ExposeEnvInfo` enabled in its security settings.

The inspect tab shows the output of the process's `HandleInspect` callback as key-value pairs. If your actor implements this method, it can expose internal state: queue lengths, cache sizes, connection counts, or any application-specific metrics. Auto-refresh polls the process once per second.

Three action buttons let you interact with the process. Send Message opens a dialog where you compose a JSON message body. Send Exit sends an exit signal with a configurable reason. Kill forcefully terminates the process. These actions are disabled for system processes.

<details>
<summary>Process detail window</summary>

<!-- screenshot: process detail floating window -->

</details>

## Applications

The applications page shows all loaded applications and their lifecycle state. Each entry displays the name, whether it's running or stopped, its mode (permanent, temporary, transient), version, process count, description, and uptime. Expanding the process count reveals individual PIDs that you can click to open detail windows.

Lifecycle controls let you start a stopped application in a selected mode, stop a running application (with an optional force flag for immediate shutdown), or unload it entirely. This is useful during development when you need to restart a misbehaving application without restarting the entire node. Start, stop, and unload are disabled for system applications to prevent breaking the node.

<details>
<summary>Applications page</summary>

<!-- screenshot: applications page -->

</details>

## Events

The events page shows all registered events on the node. Each row includes the event name, the producer process, when the event was registered, subscriber count, and message statistics: published, local sent, remote sent, and a fanout ratio showing delivery efficiency. Delta indicators highlight which events are actively publishing.

The default sort is by registration time, newest first. The Scope panel controls pagination: First shows the oldest registered events, Last shows the newest. Filters narrow by name, notify mode, buffered mode, and minimum subscriber count.

Three toggle buttons in the toolbar control how the Registered column displays timestamps: 24h/12h clock format, raw millisecond timestamps for precise correlation, and an optional date prefix. These settings are shared with the Log and Tracing pages.

<details>
<summary>Events page</summary>

<!-- screenshot: events page -->

</details>

## Network

The network page shows how the node connects to the rest of the cluster.

The connected nodes section lists every active connection with summary statistics: messages and bytes in each direction. Expanding a connection reveals negotiated flags (which features both nodes support), handshake and protocol versions, connection pool size, and per-connection traffic stats. If you need to verify whether tracing or compression is actually enabled between two specific nodes, this is where you look.

The acceptors section lists network listeners with their addresses and TLS configuration. The cluster nodes section shows all nodes known through the registrar or active connections, giving you a picture of the cluster topology. The registrar section shows the service discovery backend details.

A connection list table with its own scope controls shows all connections in a sortable, filterable view with delta indicators for message and byte counts.

<details>
<summary>Network page</summary>

<!-- screenshot: network page with expanded connection -->

</details>

## Log

The log page captures log messages in real time from every source on the node: processes, meta processes, the node itself, and the network stack.

Each log entry shows a timestamp, severity level with a color-coded badge, the source identity, and the message text. The source has a compact mode (just the PID) and a rich mode (includes behavior type and registered name) for easier identification without opening a separate detail window.

The Scope panel controls what the server captures. Level toggle buttons let you enable or disable each severity independently. This is server-side filtering: disabling debug means the server stops collecting debug messages entirely, reducing overhead on the node. A message pattern filter does server-side substring matching, with an exclude mode to filter out noise. The limit controls the ring buffer size.

The Play/Pause button stops log capture without disconnecting. When you spot something interesting, pause and read through existing entries without new messages pushing them away.

When the server drops messages because the ring buffer is full, a suppressed count indicator appears in the toolbar. If you see this frequently, increase the limit in the scope panel.

<details>
<summary>Log page</summary>

<!-- screenshot: log page -->

</details>

## Tracing

The tracing page shows distributed traces captured by Observer's tracing exporter. When you open this page, Observer registers itself as a tracing exporter and begins receiving observations. For background on how tracing works, see [Distributed Tracing](distributed-tracing.md).

Each row in the trace list shows an abbreviated trace ID, span count, root process, message type, start time, and duration. Clicking a trace expands it into a waterfall timeline. The waterfall positions all spans on a shared time axis with colored markers: blue for Sent, green for Delivered, orange for Processed. Parent-child relationships are visible through indentation, so you can follow the causal chain from the initial message through every downstream hop.

Clicking a span reveals its full detail: every field, custom attributes, and the error message if present. This is where you identify which hop introduced latency or which process returned an error.

Because Observer connects to one node at a time, it shows only the observations emitted on that node. A trace spanning three nodes will show the local observations and the remote portions as gaps. For complete cross-cluster traces, use [Pulse](../extra-library/applications/pulse.md) with Grafana Tempo or Jaeger.

The waterfall compensates for clock skew between nodes using measurements from active network connections. This keeps the timeline consistent even when node clocks differ slightly.

The Scope panel filters by span kinds (Send, Request, Response, Spawn, Terminate), observation points (Sent, Delivered, Processed), message pattern, and ring buffer size.

Expanded traces stay open when you switch to another page and come back.

<details>
<summary>Tracing page with waterfall</summary>

<!-- screenshot: tracing page with expanded waterfall -->

</details>

## Profiler

The profiler page provides on-demand snapshots of the Go runtime.

The goroutines view captures a stack dump and groups goroutines by their call stack. If 500 goroutines are all blocked on the same channel receive, they appear as one group with count 500 rather than 500 individual entries. Each group shows the count, state (running, IO wait, channel receive, select), wait duration, and originating function. Expanding a group reveals the full stack trace.

This is how you diagnose deadlocks and blocking. Filter by state to isolate goroutines stuck in "chan receive". Search by package name to find goroutines from specific actors. A large group with a long wait time in a state that should be transient usually points directly at the problem.

The heap view captures a memory allocation profile sorted by in-use bytes. Each record shows in-use bytes and objects, total allocated bytes and objects, and the allocation stack trace. Summary statistics show total in-use memory, total objects, and GC CPU fraction.

Use the heap view when memory grows unexpectedly. The allocation stack traces show exactly which code paths are responsible. If a single function dominates the in-use bytes, that is your starting point.

Both views refresh on demand rather than continuously, avoiding the overhead of constant profiling.

<details>
<summary>Profiler</summary>

<!-- screenshot: profiler goroutines view -->

</details>
