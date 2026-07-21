---
description: Real-time inspection and management of Ergo nodes
---

# Inspecting With Observer

A running Ergo node is not one program you can step through with a debugger. It is thousands of processes, each on its own goroutine, each handling its own messages, being restarted by supervisors as failures happen. You cannot pause one and read its variables the way you would a function, and adding print statements changes the very timing you are trying to understand.

Observer takes a different approach. It watches the live node the way the system itself does: what each process is doing right now, how messages travel between them, and where CPU and memory go. It embeds into the node, streams updates to your browser as they happen, and needs no changes to the processes it observes. And it is not read-only: from the same screen you can change a setting, send a message, or stop a process on the running system.

You deploy Observer on a single node and inspect the whole cluster from it. Every Ergo node runs the built-in `system` application, which is what Observer talks to, so switching to another node needs nothing extra deployed there. For installation and options, see [Observer Application](../extra-library/applications/observer.md).

To try it against a live cluster:

```bash
git clone https://github.com/ergo-services/examples
cd examples/observability
make up
```

This starts a multi-node cluster with Observer, tracing, health probes, Prometheus metrics, and Grafana dashboards. Open `http://localhost:9911` for Observer and `http://localhost:8888/dashboards` for Grafana.

## How to read this page

The tour goes from the outside in. First the node at a glance, then its applications, then finding the one process that matters among thousands, then everything about that process and how to act on it. After that come the specialized views, each answering a specific question: how messages flow, how the node is connected, what the logs say, where memory and time go, and how a single request travels across the system. The last section shows how to point any of these views at any node in the cluster.

## The node at a glance: Info

<figure><img src="../.gitbook/assets/observer.png" alt="Observer Info page"><figcaption></figcaption></figure>

The Info page answers the first question you ask: is this node healthy, and what is it running? Glance cards and live charts across the top track the numbers that move: how many processes exist and how many are running or blocked in a call, event throughput per second, memory used against live heap, and CPU split into user and system time. A climbing memory line or a growing process count shows up here first.

Below the glance the node identifies itself (name, framework version, operating system and architecture) beside two controls that take effect immediately: the [log level](../basics/logging.md), and the [tracing](distributed-tracing.md) sampler that decides whether the node starts new traces for messages sent through `node.Send()` and `node.Call()`. Turn the sampler up to investigate, and back off when you are done.

The rest of the page summarizes the node without leaving it: registry counts (registered names, aliases, events), delivery errors split into local and remote, log volume per level, the tracing spans recorded and any exporters registered, and the node's cron jobs with their schedules. A collapsible Build Info panel shows exactly which binary is running: the main module and Go version, the source revision (marked "modified" if the working tree was dirty at build time), the build settings, the full dependency list, and any local `replace` directives. That panel is often the quickest way to confirm you deployed what you think you did.

<figure><img src="../.gitbook/assets/placeholder.svg" alt="Screenshot needed"><figcaption>Screenshot needed: the Info page Build Info panel expanded, showing the main module and Go version, the source revision with the "modified" badge, and the dependency list.</figcaption></figure>

## Managing applications: Applications

An Ergo node runs its work as [applications](../basics/application.md), each a supervised group of processes. The applications page lists them with their state (loaded, running, stopping), their [mode](../basics/application.md#application-modes), and their process group, and lets you act on the lifecycle: start an application in a chosen mode, stop it, force-stop it when a graceful stop will not complete, or unload it. These are the coarse controls; when something is wrong at the application level, this is where you intervene. System applications are protected from these actions.

<figure><img src="../.gitbook/assets/placeholder.svg" alt="Screenshot needed"><figcaption>Screenshot needed: the Applications page, listing applications with their state and mode badges, their process group, and the start, stop, force-stop, and unload controls.</figcaption></figure>

From an application you can open its process tree in a floating window, and the tree is more than a diagram. You can color it as a heatmap by a chosen signal, so an entire application lights up by mailbox depth, message latency, utilization (the share of a process's lifetime spent inside callbacks), throughput, or state. A hot or backlogged branch becomes obvious at a glance, which is how you narrow a large application down to the part that is actually struggling.

<figure><img src="../.gitbook/assets/placeholder.svg" alt="Screenshot needed"><figcaption>Screenshot needed: an application's process tree in a floating window colored as a heatmap (for example by mailbox depth), with the heatmap mode selector visible.</figcaption></figure>

## Finding the process that matters: Processes

The processes page is where you spend most of your time. Every process on the node appears in a table that refreshes every second, and the columns are less a list of facts than a set of signals you learn to read.

- **Mailbox depth** turning yellow then red is a backlog forming: messages are arriving faster than the process handles them.
- A **state** stuck in "running" for tens of seconds is a process blocked inside a handler, usually on something it should not be doing there.
- High **running time** relative to uptime marks a hot process that spends its life executing handlers rather than waiting.
- A green **delta** like "+42" next to Messages In shows who is active right now, in the last second.

Because the columns are signals, sorting turns the table into a diagnostic tool. Sort by Messages In to find the busiest processes, by Mailbox to bring the most backlogged to the top, by Running Time to see where handler time goes. The columns cover identification (PID, name, behavior, application), messaging (messages in and out, mailbox depth, latency), and lifecycle (running time, init time, wakeups, uptime, state), which is enough to answer most questions without opening a single process.

Above the table, a few charts summarize the current scope rather than one process: messages in and out, the distribution of utilization across the scoped processes, and mailbox latency. A State Distribution bar shows how the scope splits across process states, and clicking a segment filters the table to it. Click any PID to open a floating detail window.

### Choosing what the table shows: Scope

A node can run tens of thousands of processes, so the table never shows all of them at once. The Scope panel decides what the node sends to the browser, and it works in two modes.

In the default mode you pick a window into the process list: "first 500" returns the 500 oldest processes, "last 500" the 500 newest, and entering a PID starts the window there. The node scans only that window, which stays fast no matter how many processes exist because it never looks beyond the range you asked for.

The "All" mode switches to a full scan: the node walks every process, applies your filters as it goes, and returns up to 10,000 matches. Because a full unfiltered scan could flood the browser, this mode requires at least one filter.

Filters narrow by name, behavior type, application, state, or minimum mailbox depth, and appear as removable chips in the toolbar. A separate search field adds quick regex matching on top of the returned rows, for ad-hoc lookups without changing the scope.

<details>

<summary>Processes page with scope panel</summary>

<figure><img src="../.gitbook/assets/processes.png" alt="Processes page"><figcaption></figcaption></figure>

**Mailbox.** Total messages across all four mailbox queues (Main, System, Urgent, Log). Changes color as the queue grows: yellow for moderate, red for deep backlog.

**Latency.** Time between a message entering the mailbox and the process starting to handle it. High latency means the process has a backlog and incoming messages are waiting. Requires the `latency` build tag to be enabled (see [Debugging](debugging.md)).

**Running Time.** Total time spent inside handler callbacks (HandleMessage, HandleCall). High running time relative to uptime means the process spends most of its life executing handlers, whether due to computation or blocking I/O.

**Init Time.** Time spent in the `Init` callback during startup. Highlighted red if over one second. Keep initialization fast: spawn has a timeout, and under a supervisor a slow Init blocks the restart of sibling processes.

**Wakeups.** How many times the process was activated to handle messages. Each activation processes one batch from the mailbox. A high wakeup count with low message counts can indicate many small deliveries.

</details>

## Everything about one process: Process Details

Once the table points you at a suspect, the floating detail window tells you everything about it. Several can be open at once, and they survive page switches, so you can keep a problem process in view while you check logs or traces elsewhere.

The **overview** tab shows two live charts: incoming and outgoing message rates over the last minute (with a rate or cumulative toggle), and the depths of the four mailbox queues (Main, System, Urgent, Log). Cards below show running time, init time, and uptime, the same signals as in the table but plotted over time. The parent and leader processes appear as links that open their own windows.

The **relations** tab reveals how the process is wired into the rest of the system: the aliases it registered, the meta processes it owns, the events it created, and its links and monitors grouped by type (see [Links and Monitors](../basics/links-and-monitors.md)). This answers a question that matters before you touch anything: who else is affected if this process terminates. From here you can also open its supervision subtree as a floating tree, colored as a heatmap by the same signals as the application tree, to see where it sits and which branch below it is busy.

The **inspect** tab shows whatever the process chooses to publish about itself through its `HandleInspect` callback, as live key-value pairs refreshed once a second. If your actor implements that method, it can surface any internal state you care about: queue lengths, cache sizes, open connection counts, the current step of a job. This is your own window into a process that the framework cannot see on its own.

<figure><img src="../.gitbook/assets/placeholder.svg" alt="Screenshot needed"><figcaption>Screenshot needed: a process detail window on the inspect tab, showing custom HandleInspect state as live key-value pairs.</figcaption></figure>

### Acting on a live process

The detail window is also where Observer stops being read-only.

The **config** tab changes settings that take effect on the running process immediately: raise its log level to get more detail from just that process, enable compression or adjust message priority and delivery guarantees for its network messages, or turn on the tracing sampler for a targeted look. The environment variables section appears when the node has `ExposeEnvInfo` enabled in its security settings.

Three actions let you intervene directly. **Send Message** delivers a string message to the process. **Send Exit** sends an exit signal with a reason you choose. **Kill** terminates it. These act on the real system, so use them deliberately; they are disabled for system processes.

<figure><img src="../.gitbook/assets/placeholder.svg" alt="Screenshot needed"><figcaption>Screenshot needed: a process detail window on the config tab, showing the live settings and the Send Message, Send Exit, and Kill actions.</figcaption></figure>

<details>

<summary>Process detail window</summary>

<figure><img src="../.gitbook/assets/process_info.png" alt="Process detail window"><figcaption></figcaption></figure>

</details>

## Watching messages flow: Events

Processes rarely work alone; they publish and subscribe to [events](../basics/events.md). The events page makes that traffic visible.

Like the processes page, it shows only what the scope defines. Each row names the event, its producer process, when it was registered, how many subscribers it has, and publication statistics, with delta indicators marking events that are actively publishing. The From control chooses First (oldest registered) or Last (newest), and filters narrow by name, notify mode, buffered mode, and minimum subscriber count.

Beyond the table, you can open a **live stream** of a single event and watch the actual messages as the producer publishes them, in real time. Filters match the message type and its content, with an exclude mode to hide noise, so you can answer questions the counters cannot: what exactly is this producer emitting, and does it match what subscribers expect. It is a live tap on production message traffic, with no change to the producer or its subscribers.

<figure><img src="../.gitbook/assets/placeholder.svg" alt="Screenshot needed"><figcaption>Screenshot needed: the live event stream window, showing messages published to a named event in real time, with the type and content filters.</figcaption></figure>

<details>

<summary>Events page</summary>

<figure><img src="../.gitbook/assets/events.png" alt="Events page"><figcaption></figcaption></figure>

**Published.** Total number of times PublishEvent was called by the producer. Each call increments this counter once regardless of how many subscribers receive the message.

**Local Sent.** Total messages delivered to local subscribers. If one publish reaches 5 local subscribers, this increments by 5.

**Remote Sent.** Total messages sent to remote nodes. Counted per remote node, not per subscriber. If a remote node has 10 subscribers, this increments by 1 because the framework uses [shared subscriptions](pub-sub-internals.md#network-optimization-shared-subscriptions) to send one message per node.

**Fanout.** Ratio of Local Sent to Published. Shows the average number of local deliveries per publish. A fanout of 3.0 means each publish reaches about 3 local subscribers.

**Buffer.** Current messages in the event's ring buffer / buffer capacity. [Buffered events](pub-sub-internals.md#buffered-events-partial-optimization) retain recent messages so that new subscribers receive catch-up data. Yellow highlight if the buffer has pending messages.

**Notify.** Whether the producer receives [notifications](pub-sub-internals.md#producer-notifications) (`MessageEventStart`/`MessageEventStop`) when the first subscriber arrives or the last subscriber leaves.

</details>

## How the node is connected: Network

The network page shows how the node reaches the rest of the cluster and how much traffic flows.

The top section is configuration: mode, max message size, handshake and protocol versions, and the negotiated flags. The registrar section shows the service discovery backend and its capabilities, and the acceptors section lists the node's network listeners with their addresses, TLS configuration, and per-acceptor flags. Below that, the page splits into three tabs.

The **Connections** tab is the default. Four live charts plot aggregate traffic across all connections: messages per second, bytes per second, compression operations, and fragmentation operations, each in and out. A connection table with its own scope shows every connection with delta indicators; click a row for a detailed window.

The **Routes** tab shows configured static routes and proxy routes side by side: static routes tell the node where to dial when a name matches, proxy routes describe how to reach nodes through an intermediary.

The **Types** tab is a snapshot of the wire-format type registry, the set of message types the node knows how to serialize. Each row shows its registration ID, owning protocol version, kind, the wire size of a zero value, and canonical name; expand a row for the inferred Go shape of the type. Two filters narrow by name and by schema content. Refresh re-fetches the registry, which rarely changes after startup, so this tab does not stream. With the node built using `-tags=typestats`, extra columns show per-type encode and decode counts and wire-byte totals; see [The typestats Tag](debugging.md#the-typestats-tag) for using them to pick compression candidates.

<figure><img src="../.gitbook/assets/placeholder.svg" alt="Screenshot needed"><figcaption>Screenshot needed: the Network page Types tab, showing the wire-format type registry with one row expanded to reveal a type's inferred schema.</figcaption></figure>

The cluster nodes section lists all nodes known through the registrar or active connections, a quick picture of the topology.

<details>

<summary>Network page</summary>

<figure><img src="../.gitbook/assets/network.png" alt="Network page"><figcaption></figcaption></figure>

**Node.** Contains several elements: a direction arrow, the node name, a CRC32 badge, and a TLS badge. The blue arrow (up-right) means the connection was initiated by this node (outgoing). The green arrow (down-left) means the connection was accepted from the remote node (incoming). The badge shows "TLS" if the connection uses TLS or "Plain" if it does not.

**Node Uptime / Connection Uptime.** Node uptime is how long the remote node has been running. Connection uptime is how long this specific connection has been active. If the connection was recently re-established after a network issue, connection uptime will be shorter than node uptime.

**Pool.** Number of TCP connections in the ENP protocol pool for this logical connection. Higher pool size allows more parallel message delivery.

**Reconnections.** How many times the connection was re-established. Non-zero values are highlighted in red. Frequent reconnections may indicate network instability.

**Clock Skew.** Measured difference between the local and remote node clocks. Used by the tracing waterfall to compensate for clock drift when displaying cross-node traces.

</details>

### Connection details

Clicking a connection row opens a window with the full picture of one connection. Metric cards show messages and bytes in each direction. The identity section shows node and connection uptimes, framework and protocol versions, max message size, and the negotiated network flags as colored pills (Remote Spawn, Fragmentation, Important Delivery, and so on), each green when both nodes agreed to enable it. Below, the pool size and reconnection counter appear, and for outgoing connections the Pool DSN lists the addresses of the pooled TCP connections.

Two live charts track messages and bytes per second each way, plus a third for transit throughput if the connection carries proxy traffic. The compression and fragmentation sections show how many messages were compressed or fragmented, the ratio, and the bytes saved, which tells you whether those features are helping or adding overhead. A "Switch observer to this node" button re-points Observer at the remote node.

<details>

<summary>Connection detail window</summary>

<figure><img src="../.gitbook/assets/connection.png" alt="Connection detail window"><figcaption></figcaption></figure>

</details>

## What the node is saying: Log

The log page captures log messages in real time from every source on the node: processes, meta processes, the node itself, and the network stack.

Each entry shows a timestamp, a color-coded severity, the source, its registered name and behavior, and the message. The source column tells you where the message came from (a process PID, a meta-process alias, the node, or a network peer), and with the rich source toggle it becomes clickable, opening the detail window for whatever generated it. Long messages collapse to three lines and expand on click, and structured fields appear as key=value pairs beneath the text.

The Scope panel controls what the node captures, and the filtering happens on the server: disabling the debug level means the node stops collecting debug messages entirely, so filtering reduces load rather than just hiding rows. You can also match on source, behavior, field names and values, and message text, with an exclude mode to remove noise, and set the ring-buffer size with the limit.

The Play/Pause button stops the stream without disconnecting, so you can freeze the view and read what is there while the node keeps running. If the buffer fills and the node has to drop messages, a suppressed-count alert appears; raise the limit if you see it often.

<details>

<summary>Log page</summary>

<figure><img src="../.gitbook/assets/log.png" alt="Log page"><figcaption></figcaption></figure>

</details>

## Where memory and time go: Profiler

The profiler answers the two questions the process table cannot: why is memory growing, and why is something stuck. A GC Pressure section stays visible at the top with four live charts, allocation rate, dead rate, live ratio, and the fraction of CPU spent in garbage collection, so you can see memory pressure building before it becomes a problem.

### Heap

The Heap tab updates continuously and lists allocations sorted by in-use bytes: for each, the in-use and total bytes and objects, and the function responsible (the first non-runtime function in the allocation stack). Expand a row for the full stack trace, or switch from the table to the flamegraph to see which call paths own the memory by area rather than by row. Filter by function name and pause to study a frozen snapshot. Reach for this when memory grows unexpectedly: the stacks point straight at the code paths doing the allocating, and if one dominates the in-use bytes, that is where to start.

### Goroutines

The Goroutines tab captures a snapshot on demand when you press Capture. The table view groups goroutines by call stack, so 500 goroutines blocked on the same channel receive show up as one group of 500, with the state (running, chan receive, select, sleep, and so on), how long they have waited (green under a minute, yellow under five, red beyond), and where each was spawned versus where it is now. Expand a group for the full stack and goroutine IDs, or switch to the flamegraph view for the same snapshot arranged by stack. Filter the capture by stack content, state, and minimum wait time.

This is how you find deadlocks and blocking. Filter to "chan receive", search for a package name to isolate a specific actor, and a large group stuck for a long time in a state that should be brief usually points right at the problem. Both views work without a restart or any special build flag.

<figure><img src="../.gitbook/assets/placeholder.svg" alt="Screenshot needed"><figcaption>Screenshot needed: the Profiler flamegraph view (Heap or Goroutines), showing allocation or goroutine stacks as a flame graph.</figcaption></figure>

<details>

<summary>Profiler</summary>

<figure><img src="../.gitbook/assets/profiler.png" alt="Profiler"><figcaption></figcaption></figure>

</details>

## Following a request across the system: Tracing

The hardest question in a message-passing system is what actually happened when a request came in, because the work spreads across many processes and often many nodes. Tracing answers it. When tracing is on, Observer collects traces continuously, so data is already waiting when you open the page. For how tracing works, see [Distributed Tracing](distributed-tracing.md).

Because Observer connects to one node at a time, it shows the observations recorded on that node. For a single trace stitched across the whole cluster, export to [Pulse](../extra-library/applications/pulse.md) with Grafana Tempo or Jaeger.

### Trace list

Traces are listed newest first. Each row shows a copyable trace ID, the root process and the message that started the trace, an error marker if any span failed, the span count, a bar showing this trace's duration against the longest in view, and the total duration. The search field matches across trace and span fields alike (IDs, from, to, message text, attributes), Pause holds new traces back, and Clear empties the buffer.

### Waterfall

Click a trace to expand its waterfall. It groups the observation points for each message (Sent, Delivered, Processed) into one row and arranges the rows into a tree by parent and child, so the indentation is the causal chain of who triggered whom.

Each row carries a color-coded kind (SEND, CALL, RESP, SPAWN, TERM), the sender and receiver with their behaviors, the message type, and a timeline bar split into a lighter transit segment (Sent to Delivered) and a solid processing segment (Delivered to Processed). Hovering shows the node at each point and the exact durations. For a message that crosses nodes, the transit time subtracts the measured clock skew between them, so the number reflects real travel time rather than clock drift. Local PIDs are clickable and open detail windows, and clicking a row opens a panel with every field of the span and the custom attributes merged from all of its observation points.

### Scope

The Scope panel toggles which span kinds (SEND, CALL, RESP, SPAWN, TERM) and observation points (Sent, Delivered, Processed) are collected, and a message pattern filter matches message type and error text with an optional exclude. The buffer limit sets how many traces are kept.

<details>

<summary>Tracing page with waterfall</summary>

<figure><img src="../.gitbook/assets/tracing.png" alt="Tracing page with waterfall"><figcaption></figcaption></figure>

</details>

## Inspecting the whole cluster

Everything above applies to one node, but you rarely run just one. The sidebar has a node selector listing every node Observer can see through the registrar; pick one and all the views above re-point to it. Nothing needs to be deployed on the target: it already runs the `system` application that Observer talks to, and the list updates live as nodes join and leave the cluster.

You can also reach a node that Observer is not yet connected to. If the registrar knows it, selecting it is enough; otherwise you provide its address (host, port, and cookie, optionally over TLS) and Observer establishes the connection. The "Switch observer to this node" button on a connection detail window does the same for a peer you are already looking at. From a single browser tab, you move freely across the entire cluster.

<figure><img src="../.gitbook/assets/placeholder.svg" alt="Screenshot needed"><figcaption>Screenshot needed: the sidebar node selector listing cluster nodes, alongside the connect-to-node dialog for reaching a node by address.</figcaption></figure>
