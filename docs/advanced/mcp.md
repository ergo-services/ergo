---
description: What your AI agent can do with a live Ergo cluster over MCP
---

# Inspecting With an AI Agent

[Observer](../extra-library/applications/observer.md) serves an MCP endpoint beside its web UI. Point Claude Code, Cursor or any MCP-compatible client at it, and your agent can inspect and operate the running cluster directly.

What that changes in practice: instead of opening a dashboard and deciding which panel to look at, you describe the symptom - "orders are slow since the deploy" - and the agent goes and looks. It lists the nodes, finds the processes with deep mailboxes, reads the one that looks implicated, asks it about itself, follows the message chain onto the next node, and comes back with a cause rather than a screenshot.

Three things make it worth wiring up:

- **One endpoint covers the whole cluster.** Only the node running Observer needs it. Every other node is already inspectable, because each runs the framework's built-in `system` application - nothing to install, no port to open, no agent to deploy.
- **The agent reads the real thing.** The same counters, mailboxes, links, logs and profiles the framework maintains for itself, not a metrics summary sampled a minute ago.
- **It can act, if you let it.** Set a log level, send a message, restart an application, retune a process on the fly. All of it behind a permission model you configure, and off by default on a read-only listener.

## Connecting

The endpoint is `/mcp` on any listener that serves it, which by default is the same one serving the UI:

```bash
claude mcp add --transport http ergo http://localhost:9911/mcp
```

Behind a proxy that authenticates, pass what it expects:

```bash
claude mcp add --transport http ergo http://localhost:9911/mcp \
  --header "Authorization: Bearer ${TOKEN}"
```

That is the whole setup. The surface tells the client where to start and what it may do, so there is no tool list to maintain on your side.

Worth setting once: `Instructions` in [The MCP surface](../extra-library/applications/observer.md#the-mcp-surface). It is where you write down what no amount of inspection reveals - which node runs which part of the business, where a flow begins, what must not be touched. The agent is told this before it asks anything.

## What your agent can ask for

Thirty-eight tools. Twenty-nine read, nine change something. Every tool that asks about a node takes a `node` argument, so any question can be aimed at any node in the cluster. The four that are not about a single node are the exception: `cluster_query` takes a list of nodes, `cluster_batch` names a node per step, and `job_list` and `job_cancel` are about runs.

**The node itself**

| Tool | What it gives you |
|------|-------------------|
| `node` | What the node is right now: uptime, memory, process and application counts, version, environment |
| `network` | The network as configured and as running: acceptors, flags, registrar, whether the stack is stopped |
| `connections` | Every peer this node is connected to, what was negotiated with each, how much has crossed it |
| `connection` | One peer in full: agreed flags, the connection pool, bytes and messages each way, whether it has since dropped |
| `capabilities` | What this observer may do on that node - what the node offers, crossed with what the caller is allowed |

**Processes**

| Tool | What it gives you |
|------|-------------------|
| `processes` | Every process on the node: what it is, what it runs under, how much it has handled, what it is doing now |
| `process_state` | What one process says about **itself** - the sections its behavior chooses to expose, such as a supervisor's restart count |
| `process_lookup` | Whether a process is alive and what it is now, by registered name or by id |
| `subtree` | The processes below one supervisor |
| `meta_state` | The state of one meta process - a socket, a listener, a stream |

The full framework-level record of a single process is a reading rather than a tool: `ergo://<node>/process/<pid>`, with its mailbox depths and latencies, links, monitors, aliases and delivery settings.

**Applications**

| Tool | What it gives you |
|------|-------------------|
| `applications` | The applications on the node, running or merely loaded, with mode, weight, published roles and process count |
| `app_tree` | Every process running under one application |

**Events (pub/sub)**

| Tool | What it gives you |
|------|-------------------|
| `events` | Every event on the node: who produces it, how many subscribe, whether it buffers, who may publish |
| `event` | One event: its producer, its buffer, subscriber count, when it last published |

**Diagnostics that cost something**

| Tool | What it gives you |
|------|-------------------|
| `goroutines` | A goroutine dump, filterable by stack text, state or wait time |
| `heap_profile` | What is allocated and by which call path |

Both stop the world on the node they run on for as long as the walk takes. Ask once, read carefully, never in a loop and never fanned out across a cluster.

**Scheduling**

| Tool | What it gives you |
|------|-------------------|
| `cron` | The cron jobs: spec, timezone, when each last ran and what it left behind |
| `cron_schedule` | What the node will run, and when |

**Service discovery**

| Tool | What it gives you |
|------|-------------------|
| `registrar_nodes` | Every node registered with the service registry this node uses |
| `registrar_routes` | How to reach one node: host, port, TLS, the versions it speaks |
| `registrar_application_routes` | Which nodes publish one application, with the mode and state each reports |
| `registrar_proxy_routes` | Which node relays to another when it cannot be reached directly |

**The wire**

| Tool | What it gives you |
|------|-------------------|
| `types` | The message types registered for the network, with per-type encode and decode counters |
| `errors` | The sentinel errors that node can carry over the network |
| `atoms` | The atoms it keeps in its wire cache, sent as an id instead of a string |

These answer the question a distributed-systems bug eventually raises: can these two nodes actually understand each other.

## Readings it can return to

Anything above that describes a whole thing is also addressable, as `ergo://<node>/<lens>` - the node, its processes, its network, one process, one event. The agent reads an address, and reads the same address again later to see what moved, which is how it watches a mailbox drain instead of guessing.

Three of those readings accumulate rather than answer once:

| Reading | What arrives |
|---------|--------------|
| `ergo://<node>/log` | The lines the node logs, filtered by level or pattern on the node itself |
| `ergo://<node>/stream/{event}` | The actual messages flowing through one event |
| `ergo://<node>/tracing` | The spans the node emits while tracing is on |

They start collecting when first read, so the first answer is usually near-empty and the second one has what happened in between. This is what makes "watch it and tell me when it recovers" a thing an agent can actually do.

## One question across the cluster

| Tool | What it does |
|------|--------------|
| `cluster_query` | Asks one tool of many nodes at once, in parallel |
| `cluster_batch` | Runs different questions on different nodes at once - each step names its own node, tool and arguments |
| `job_list` | The runs you still hold |
| `job_cancel` | Stops one |

Both answer immediately with the address of a run, and the answers land as nodes report. A slow node does not hold up the others, and an unreachable one comes back as refused rather than waited for. `ergo://cluster` is the cheaper question when all you need is who is up, how long they have been up, and who fell out and why.

## Letting it act

| Tool | What it does |
|------|--------------|
| `send` | Delivers one message to a process or a meta |
| `send_exit` | Asks a process to stop, so it runs `Terminate` |
| `kill` | Terminates a process at once, without letting it run `Terminate` |
| `log_level_set` | Changes the log level of the node, one process or one meta |
| `tracing_sampler_set` | Turns tracing on or off, for the node or for one process |
| `process_tune` | Changes one delivery setting of a running process: send priority, compression, message ordering, important delivery |
| `app_start` | Starts an application already loaded on the node |
| `app_stop` | Stops a running application |
| `app_unload` | Unloads it, so the node no longer knows it |

`process_tune` and `tracing_sampler_set` are the two that change the shape of an investigation: a hypothesis about compression or priority gets tested on the live system, and tracing gets switched on for the one process that matters, without a deploy.

## Keeping it bounded

The permission model is the listener's, not the agent's. Set `Ceiling: observer.Ceiling{ReadOnly: true}` and the nine tools above are not merely refused - they are not offered, so an agent never plans around them. A finer ceiling can deny individual operations, or restrict which nodes are reachable at all.

Two habits are worth asking of an agent operating production: call `capabilities` before planning anything that writes, and act on one thing at a time. A kill is not reversible, and fanning a mutating tool across the cluster is deliberately not offered.

For the full configuration - listeners, surfaces, ceilings, authorizers - see [Observer](../extra-library/applications/observer.md).

## An investigation, end to end

A symptom in plain words: orders are slow since the deploy. What follows is the shape of the work.

**Where are we.** `ergo://cluster`: fifteen nodes, three named `orders@*`, one restarted four minutes ago. That restart is the first thing worth explaining.

**Ask all three at once.** `cluster_query` with `processes` over the `orders@*` nodes, narrowed to processes holding a real backlog. Two look ordinary. The restarted one has a process with a mailbox in the thousands.

**Read that process.** `ergo://orders-2@host/process/<pid>`: the mailbox is deep and it has been running for tens of seconds.

**Ask it about itself.** `process_state` on the same pid - and here the actor names the upstream it is waiting on, which no generic reading would have told you.

**Follow it.** The upstream is a process on another node. Modest mailbox, high running time, its own state pointing at an external call. Nothing is queueing there; it is simply slow, and everything behind it is queueing.

**Confirm.** `goroutines` on that node, once, filtered. The stacks are parked in the external call. That is the cause: not the node, not the framework, a dependency that got slower.

**Watch it recover.** Read the process listing and the log again as the dependency comes back, and watch the mailbox drain.

Nothing in that sequence was decided in advance. Each step chose the next from what the previous one answered.

## See also

- [Observer](../extra-library/applications/observer.md) - adding it to a node, and bounding what an agent may do
- [Inspecting With Observer](observer.md) - the same node through the web UI
- [AI Agents](../ai-agents.md) - building agents with the framework, and diagnosing them
