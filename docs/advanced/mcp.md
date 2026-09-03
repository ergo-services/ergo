---
description: Inspecting and managing a live cluster through an AI agent
---

# Inspecting With an AI Agent

[Observer](../extra-library/applications/observer.md) serves the same node to two very different readers. A person gets a web UI: pages laid out in advance, each answering questions somebody decided were the important ones. An agent gets something else, because it does not read layouts and does not know in advance which question matters.

That difference is the whole design. A dashboard is a fixed set of answers; an agent needs the surface those answers were made from, so it can start at a symptom, enumerate what exists, read the state of whatever looks implicated, correlate across nodes, and narrow down. Point diagnosis becomes system diagnosis, because nothing has to be chosen up front.

This page is how that surface is shaped and how to work through it. For installing Observer and bounding what an agent may do, see [Observer](../extra-library/applications/observer.md).

## How to read this page

First the connection, then the two halves of the surface and why there are two. Then reading a single node, from the cluster down to one process, and how an answer explains its own numbers. Then the parts that exist because a cluster is not one node: one question put to many, and readings that accumulate instead of answering once. Then acting on a live system, what it costs the node, and finally a whole investigation from symptom to cause.

## Connecting

The MCP surface is served on `/mcp` by any listener that has it enabled, which by default is the same one serving the UI:

```bash
claude mcp add --transport http ergo http://localhost:9911/mcp
```

With an authenticating proxy in front, pass what it expects:

```bash
claude mcp add --transport http ergo http://localhost:9911/mcp \
  --header "Authorization: Bearer ${TOKEN}"
```

The transport is POST-only and follows the 2026-07-28 revision of the protocol: every request carries the protocol version, states what the client can accept, and names its method in a header. A compliant client does all of that for you, which is the point of using one. If you are speaking to the endpoint by hand and it answers `-32020`, a header is either missing or disagrees with the body, and the message says which one.

The first thing the surface tells a client about itself is where to start, and a deployment can add to that. See `Instructions` in [The MCP surface](../extra-library/applications/observer.md#the-mcp-surface): it is the one place to write down what no amount of inspection reveals, such as which node runs which part of the business.

## Two halves: resources and tools

Everything the surface offers is either a **resource** you read by URI or a **tool** you call by name. The split is not decoration.

A resource is a *reading*: it has an address, it can be read again, and reading it again can mean "only what has changed". That fits a node's state, a process, a log, a stream of events, a run in progress. An agent that holds a URI holds something it can come back to.

A tool is an *act*: it takes arguments, does one thing, and answers once. That fits a dump, a lookup, a question put to many nodes, and everything that changes the node.

Resource URIs are the `ergo://` scheme:

```
ergo://cluster                                  every node this observer knows
ergo://<node>                                   the node lens, the default
ergo://<node>/<lens>                            one of the lenses below
ergo://<node>/<lens>/<target>                   the ones that address something
ergo://<node>/<lens>?<params>                   filters and paging
ergo://<node>/watch/<key>/<lens>                the same lens, under a name of yours
ergo://job/<key>                                a run started by cluster_query or cluster_batch
```

`since=` is reserved on every URI: it is how a second read takes only what has landed since the first. No lens may be called `cluster`, `job` or `watch`.

## Start at the cluster

`ergo://cluster` is the entry point, and the instructions say so for a reason. It names every node the observer knows, which is where every other URI and every tool argument gets its node name. It is also the answer about the cluster as a whole: which nodes are online, how long they have been up, and which ones fell out and why.

Nothing else is addressable without a node name, and a name invented by an agent is refused rather than guessed at.

## The lenses of a node

| Lens | URI | What it answers |
| ---- | --- | --------------- |
| `node` | `ergo://<node>` | What the node is right now: uptime, memory, processes, applications, connections, version, environment. |
| `processes` | `ergo://<node>/processes` | Every process: what it is, what it runs under, what it has handled, what it is doing now. |
| `process` | `ergo://<node>/process/{pid}` | One process in full, as the framework sees it: mailbox depths and latencies, links, monitors, aliases, metas, compression and delivery settings. What the process says about *itself* is the separate `process_state` tool. |
| `meta` | `ergo://<node>/meta/{alias}` | One meta process: a socket, a listener, a stream. |
| `network` | `ergo://<node>/network` | The network as configured and as running. |
| `connections` | `ergo://<node>/connections` | Every peer, with what was negotiated with each. |
| `connection` | `ergo://<node>/connection/{peer}` | One peer in full: flags both sides agreed on, the pool behind it. |
| `applications` | `ergo://<node>/applications` | Applications loaded, running or not, with their mode and group. |
| `events` | `ergo://<node>/events` | Every event registered: producer, subscribers, publication counters. |
| `event` | `ergo://<node>/event/{name}` | What one event is, not what flows through it. |
| `stream` | `ergo://<node>/stream/{name}` | The messages flowing through one event, as they arrive. |
| `log` | `ergo://<node>/log` | The lines the node logs, from the moment the lens is first read. |
| `tracing` | `ergo://<node>/tracing` | The spans the node emits while tracing is on. |

The first ten answer with the state at the moment of reading. The last three, `stream`, `log` and `tracing`, accumulate: they start collecting when the lens is first read and hold what arrived, so a second read with `since=` gives the new part rather than the whole thing again. That is the difference between asking what a process looks like and watching what a node says.

Filters and paging are query parameters, announced per lens in the resource templates the client already has. `processes` takes `namePattern`, `behavior`, `application`, `state`, `minMailbox`, `pidStart`, `pidLimit`; `log` takes `levels`, `limit`, `messagePattern`, `messageExclude`, `since`; and so on. Two of those deserve a note: `pidStart` with `pidLimit` walk the id space in order, which makes a page repeatable, while `pidLimit: -1` asks instead for whatever is alive now, in no order, at the cost of the living rather than of every id the node has ever used.

## Reading the same lens under your own name

`ergo://<node>/watch/<key>/<lens>` is the same lens with a cursor of its own, held under a name you choose. Two agents, or two investigations by the same agent, can follow one node's log without consuming each other's position.

A key belongs to a caller, so a keyed reading needs an authorized listener: on an open one the surface answers that it needs a caller rather than silently sharing a cursor between everybody. Without a key the reading is shared, which is the right default for a question asked once.

## Every answer explains its own numbers

An answer that carries a `services.ergo/legend` key says what its own fields mean. Each entry is the path to a field from the object holding the legend, with `[]` for a step into a list:

```json
{
  "Processes": [ ... ],
  "services.ergo/legend": {
    "units": {
      "Processes[].Uptime": "sec",
      "Processes[].MailboxLatency": "ns"
    },
    "sentinels": {
      "Processes[].MailboxLatency": "-1 = built without -tags=latency, 0 = all queues empty"
    },
    "axes": {
      "LogMessages": "trace,debug,info,warning,error,panic"
    }
  }
}
```

Three kinds of thing get explained. **Units**, because a number called `Uptime` is meaningless until you know it is seconds. **Sentinels**, because `-1` in a latency field is not a fast mailbox, it is a node built without the [`latency` build tag](debugging.md). **Axes**, because a counter array is only readable if you know which cell is which level.

This exists so an agent does not have to carry a table of field meanings that drifts from the code. The legend is generated from the same declarations the values come from.

## An absent thing says why

A reading that cannot be taken is refused with a reason rather than answered with emptiness. An event that no longer exists, a process that terminated between two reads, a node that stopped publishing: each says which, and a `Refused` map names what could not be read and why.

That distinction matters more for an agent than for a person. A person seeing an empty list looks at the screen and knows something is off. An agent given an empty list concludes there is nothing there, and reasons on from a false premise.

## One question, many nodes

A cluster is not one node, and asking thirty nodes one at a time is both slow and hard to read. Two tools do the fan-out:

- `cluster_query` puts **one** question to many nodes.
- `cluster_batch` puts **a different question to each node**: one step is a node, a tool and its arguments, and the steps run in parallel. It is for when you already know which node runs which application and want one business flow in a single go.

Both answer immediately with the URI of a run, `ergo://job/<key>`, rather than waiting for every node. You read that URI to see what has landed, and read it again with `since=` to take only what arrived after the previous read. A run that is still going says so; a slow or unreachable node does not hold up the others.

`job_list` shows the runs you have, `job_cancel` stops one. A run belongs to the caller that started it: with an authorized listener nobody else can read or cancel it. Runs are bounded by `JobLimit` and expire by `JobMaxRetention`, so an abandoned investigation does not leave work behind on the node.

The key is yours to choose, and asking the same key the same question joins the run in progress instead of starting a second one. Asking the same key a *different* question is refused: joining it would hand you answers to something you did not ask.

## Following a node instead of polling it

`subscriptions/listen` is a long-lived POST that follows a set of resources and delivers `notifications/resources/updated` as they change, instead of the agent re-reading on a timer.

The stream acknowledges what it accepted before anything else, with `notifications/subscriptions/acknowledged` naming the resources it is following. If a resource stops being available, the stream says so and re-acknowledges what is left, closing when nothing remains. Cancellation arrives as `notifications/cancelled`.

Each open stream costs the listener one of its `MaxStreams`, and each subscription inside it costs a producer on the observed node, bounded by `MaxSubscriptions`.

## The tools

Thirty-eight of them. The read ones cover what the lenses do not: things that are acts rather than addressable readings.

**About the node** — `node`, `network`, `connections`, `connection`, `applications`, `events`, `event`, `processes`, `capabilities`

**About one thing** — `process_state`, `meta_state`, `process_lookup`, `app_tree`, `subtree`

**About the wire** — `types`, `errors`, `atoms`

**Expensive** — `goroutines`, `heap_profile`

**Scheduling and discovery** — `cron`, `cron_schedule`, `registrar_nodes`, `registrar_routes`, `registrar_proxy_routes`, `registrar_application_routes`

**Many nodes** — `cluster_query`, `cluster_batch`, `job_list`, `job_cancel`

**Changing the node** — `send`, `send_exit`, `kill`, `log_level_set`, `tracing_sampler_set`, `process_tune`, `app_start`, `app_stop`, `app_unload`

`capabilities` is the one to call before planning anything that writes. It answers what *this* node allows *this* caller, already narrowed by every ceiling between them, so an agent can decide what is possible instead of discovering it through refusals.

The tool list is filtered the same way. A tool the caller may not use at all is not in `tools/list`, so on a read-only surface the mutating tools are simply not offered. A tool that needs several capabilities and has only some of them refused stays in the list with the refused ones named in its description, because it is still usable for everything else.

## Acting on a live system

The mutating tools do what their names say, and every one of them is a capability under `manage.` that a ceiling can refuse. Called anyway, on a surface that does not permit it, the refusal names the capability and points at the `capabilities` tool of that node rather than failing blankly.

`process_tune` is the interesting one: it changes the per-process network knobs, send priority, compression, message ordering, important delivery, on a running process, which is how a hypothesis about a bottleneck gets tested without a deploy.

Two rules are worth stating for an agent operating a production system. Ask `capabilities` first, and act on one thing at a time: a kill is not reversible, and a cluster-wide fan-out of a mutating tool is not offered at all, which is deliberate.

## What it costs the node

Most of this surface reads counters the framework already maintains, and costs nothing worth thinking about.

Two tools do not. `goroutines` and `heap_profile` stop the world on the node they run on for as long as the walk takes. Ask for one once, read it carefully, and do not put either into a loop or a fan-out across a cluster.

An accumulating lens (`stream`, `log`, `tracing`) holds what it gathered until it is read or expires, and it gathers only while it is being watched. A subscription costs a producer on the observed node. A run holds a pool of workers until it finishes.

None of this is dangerous, and all of it is bounded by the listener's configuration. The reason to know it is that an agent asking the wrong thing in a loop is the one client capable of making the cost visible.

## An investigation, end to end

A symptom, in plain words: orders are slow since the deploy. What follows is the shape of the work, not a transcript.

**Establish where you are.** Read `ergo://cluster`. Fifteen nodes, three named `orders@*`, one of them restarted four minutes ago. That restart is the first thing worth explaining, and the node name is now something every later URI can use.

**Ask the cluster one question.** `cluster_query` with `processes` over the three `orders@*` nodes, narrowed with `minMailbox` so only the backed-up ones come back. It answers with a run URI; read it, and read it again with `since=` until the nodes have reported. Two nodes look ordinary. The restarted one has a process with a mailbox in the thousands.

**Read that process.** `ergo://orders-2@host/process/<pid>`. The mailbox is deep and the state has been `running` for tens of seconds. The legend says the latency field is `-1`, so this node was built without the `latency` tag and mailbox latency is unavailable rather than zero.

**Ask the process about itself.** `process_state` on the same pid. The resource above is what the framework knows; this is what the actor chose to expose, and it is where a supervisor's restart count or a worker's current upstream appears. Here it names the upstream it is waiting on.

**Follow what it says.** The upstream is a process on another node. Read it, then ask it about itself too: modest mailbox, high running time, and its own state points at an external call. Nothing is queueing there; it is simply slow, and everything behind it is queueing.

**Confirm rather than assume.** `goroutines` on that node, once, filtered. The stacks show the goroutines parked in the external call. That is the cause: not the node, not the framework, an external dependency that got slower.

**Watch the recovery.** Subscribe to `ergo://orders-2@host/processes` and to the log, and watch the mailbox drain as the dependency recovers, instead of re-reading on a timer.

Nothing in that sequence was decided in advance. Each step chose the next from what the previous one said, which is exactly what a page laid out ahead of time cannot do.

## See also

- [Observer](../extra-library/applications/observer.md) - installing it, and bounding what an agent may do
- [Inspecting With Observer](observer.md) - the same node through the web UI
- [Inspecting Actor State](inspecting-state.md) - what your own actors expose to any of this
- [Debugging](debugging.md) - the build tags that decide which numbers exist
- [AI Agents](../ai-agents.md) - Ergo as the runtime for the agents themselves
