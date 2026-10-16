---
description: Build, run, and diagnose multi-agent AI systems on Ergo Framework
---

# AI Agents

An LLM-backed worker can keep its conversation state in an actor and accept tasks as messages. Several workers may run in a pool; a supervisor can replace one that terminates; processes on other nodes can address them through the same messaging API.

The examples on this page apply those mechanisms to agent workloads. They also show how to expose state for inspection. An actor restart does not recover a conversation or retry a completed external operation: persistence, deadlines and duplicate handling remain part of the agent's protocol.

## Why Ergo fits AI agents

Consider a research worker that calls an LLM and keeps the findings for a later task. While that call waits, the worker cannot handle another message. Other actors can continue, but this worker needs a deadline so an unavailable service does not occupy it indefinitely.

There are two different recovery problems. A supervisor can create a replacement after the actor terminates. Recovering its findings or determining whether a tool already performed an external action requires application state outside that actor. A retry must also account for external effects that may have completed before the answer was lost.

The examples below start with one worker, add concurrency for independent tasks, and then route work to specialists. These use the same actor mechanisms described in Basics and Actors.

## Your agent as an actor

Represent the worker as an actor: a process with private state and a mailbox, handling messages sequentially.

```go
type ResearchAgent struct {
    act.Actor
    notes []string
}

type MessageResearchTask struct {
    Query   string
    ReplyTo gen.PID
}

func (a *ResearchAgent) HandleMessage(from gen.PID, msg any) error {
    switch m := msg.(type) {
    case MessageResearchTask:
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        result, err := callLLM(ctx, m.Query)
        cancel()
        if err != nil {
            a.Log().Warning("research failed: %s", err)
            return nil
        }
        a.notes = append(a.notes, result)
        replyTo := m.ReplyTo
        if replyTo == (gen.PID{}) {
            replyTo = from
        }
        if err := a.Send(replyTo, result); err != nil {
            a.Log().Warning("research answer not sent: %s", err)
        }
    }
    return nil
}

func factory_ResearchAgent() gen.ProcessBehavior { return &ResearchAgent{} }
```

In this fragment, `callLLM(ctx, query)` is the application's adapter and must honor the context deadline. Import `context` and `time` as well as the actor packages. A timeout or API error is logged without terminating the actor. A successful finding is added to `notes`, then sent to `ReplyTo` or the original sender when that field is empty.

This message protocol sends an answer only on success. If the caller needs a result for both success and failure, define a response type or use `HandleCall` and an asynchronous request. [Requests Without Blocking](advanced/async-request.md) shows that conversation in detail.

The surrounding runtime provides:

- **Panic recovery.** With recovery enabled, `act.Actor` converts a panic in its callback into actor termination. It does not isolate shared memory or make an external API call cancellable. See [Process](basics/process.md).
- **Supervision.** Put the agent under a supervisor and it restarts on failure with your chosen strategy. See [Supervisor](actors/supervisor.md).
- **Distributed addressability.** Each agent has a PID that works across nodes. See [Remote Spawn Process](networking/remote-spawn-process.md).
- **Event-based coordination.** Agents publish to and subscribe to named event streams, fanning out one network message per node instead of one per subscriber. See [Events](basics/events.md).
- **Live diagnostics.** Expose the running system to any AI assistant through the MCP surface of the [Observer](extra-library/applications/observer.md) application.

The actor's private state (`notes` in the example) needs no synchronization: messages arrive one at a time.

## Multi-agent architecture patterns

### Agent Pool

Run several workers from the same factory for independent tasks such as LLM calls or embedding requests. Avoid a pool when a task needs affinity to one worker's conversation state.

```go
type AgentPool struct {
    act.Pool
}

func (p *AgentPool) Init(args ...any) (act.PoolOptions, error) {
    return act.PoolOptions{
        PoolSize:      10,
        WorkerFactory: factory_ResearchAgent,
    }, nil
}

func factory_AgentPool() gen.ProcessBehavior { return &AgentPool{} }

// Spawn the pool
poolPID, _ := node.Spawn(factory_AgentPool, gen.ProcessOptions{})

// Send tasks. The pool forwards to an available worker automatically.
process.Send(poolPID, MessageResearchTask{Query: "Summarize Q3 report"})
```

Worker capacity is `PoolSize` active tasks plus `PoolSize × WorkerMailboxSize` queued tasks when the limit is configured. The pool itself also has a mailbox. On a saturated forward it drops asynchronous traffic and calls `HandleForwardFailed`; the original sender may have seen a successful send to the pool. See [Pool](actors/pool.md).

### Agent Router

When agents specialize by task type (research, analysis, code generation, summarization), route each incoming task to the right agent by content. Unlike a pool of identical workers, a router owns named slots of different agent types and dispatches by inspecting the message.

```go
type AgentRouter struct {
    act.Router
}

func (r *AgentRouter) Init(args ...any) (act.RouterOptions, error) {
    return act.RouterOptions{
        Routes: []act.Route{
            {Name: "research", Factory: factory_ResearchAgent},
            {Name: "analysis", Factory: factory_AnalysisAgent},
            {Name: "codegen",  Factory: factory_CodeAgent},
            {Name: "summary",  Factory: factory_SummaryAgent},
        },
    }, nil
}

func (r *AgentRouter) RouteMessage(from gen.PID, msg any) gen.Atom {
    switch msg.(type) {
    case MessageResearchTask: return "research"
    case MessageAnalyzeTask:  return "analysis"
    case MessageCodeTask:     return "codegen"
    case MessageSummaryTask:  return "summary"
    }
    return act.RouteDiscard
}
```

For sharded stateful agents (per-user conversation memory, per-tenant context), use hash-based routing into a fixed set of slots. Compose with `act.Supervisor` per slot if you need restart limits and mailbox preservation across worker crashes. See [Router](actors/router.md) for the full pattern catalogue.

### Agent Pipeline

In a pipeline, one actor sends its output to the next stage. Give each stage an explicit input and result contract, then place it under a supervisor whose policy fits the dependency between stages.

```go
// ResearchAgent forwards its result to AnalysisAgent
func (a *ResearchAgent) HandleMessage(from gen.PID, msg any) error {
    switch m := msg.(type) {
    case MessageResearchTask:
        findings := a.research(m.Query)
        a.Send(a.analysisPID, MessageAnalyze{Findings: findings, ReplyTo: m.ReplyTo})
    }
    return nil
}
```

If `AnalysisAgent` terminates and is restarted, its replacement has a new PID. Address the stage through a registered name or update the stored PID; otherwise the research worker keeps sending to the old incarnation. Restart alone does not recover a finding lost in the old mailbox. A stage can also be a pool when its tasks are independent.

### Distributed Agent Cluster

Spawn agents on specific nodes and address them with the same API as local agents.

```go
// Register the factory on the target node. Security: only named factories
// can be spawned remotely.
network.EnableSpawn("research-agent", factory_ResearchAgent)

// From any other node, get a handle and spawn
remote, _ := node.Network().GetNode("worker@otherhost")
pid, _ := remote.Spawn("research-agent", gen.ProcessOptions{})

// Send works identically whether pid is local or remote
node.Send(pid, MessageResearchTask{Query: "..."})
```

See [Remote Spawn Process](networking/remote-spawn-process.md) for the security model and application-level inheritance.

### Event-Driven Coordination

Agents communicate through named event streams. One producer, any number of subscribers on any nodes.

```go
// Producer: research agent publishes findings
token, _ := producer.RegisterEvent("research.findings", gen.EventOptions{})
producer.SendEvent("research.findings", token, Finding{Topic: "market-trends"})

// Subscribers on any nodes
process.MonitorEvent(gen.Event{Name: "research.findings", Node: "research@host"})
```

For a remote event, the producer sends one copy to each subscriber node; that node fans it out to its local subscribers. With subscribers on ten remote nodes, that is ten network deliveries plus local mailbox deliveries. This reduces network fan-out without eliminating per-subscriber work. See [Events](basics/events.md) and [Pub/Sub Internals](advanced/pub-sub-internals.md).

## Live diagnostics for AI systems

An agent may be waiting on an LLM, executing a tool, retrying a request or deciding its next action. Expose that phase, its start time and the pending operation ID through inspection. Mailbox depth shows queued work; the actor's phase helps explain why that work is waiting.

Add the [Observer](extra-library/applications/observer.md) application to your node. One listener
serves the web UI, the browser API and the MCP surface:

```go
import "ergo.services/application/observer"

node, _ := ergo.StartNode("mynode@localhost", gen.NodeOptions{
    Applications: []gen.ApplicationBehavior{
        observer.CreateApp(observer.Options{Port: 9911}),
    },
})
```

Connect Claude Code (or any MCP-compatible client):

```
claude mcp add --transport http ergo http://localhost:9911/mcp
```

Now you describe a symptom in plain English and the AI works the live system:

```
You: "Why is the order processing agent slow?"

AI: reading ergo://cluster ... three orders@* nodes, one restarted 4m ago
    cluster_query processes over the three, minMailbox=100
    -> orders-2: order_processor holds 847 messages (the others hold single digits)
    reading ergo://orders-2@host/process/<pid>
    -> running for 30s, mailbox latency unavailable (built without -tags=latency)
    process_state on that pid
    -> the actor's own sections name payment_validator upstream
    reading that process on billing-1, then asking it about itself
    -> small mailbox, high running time, its state points at an external call
    goroutines on billing-1, once, filtered
    -> parked in external_api.Call(). The payment API is the bottleneck.
```

This is an illustrative investigation. Each observation narrows the next query: first locate the backlog, then inspect the waiting actor and its dependency, then check the stack on the node running that dependency. A useful `HandleInspect` makes the intermediate steps possible.

The surface has two halves. A **resource** is a reading with an address: `ergo://<node>/<lens>`,
read again with `since=` to take only what has landed since. A **tool** is an act that answers
once: 38 of them, from a process listing to a heap profile, plus two that reach the whole cluster -
one putting a single question to many nodes, the other a different question to each. Answers explain their own numbers, so the agent needs no table of
field meanings that drifts from the code, and a reading that cannot be taken is refused with a
reason rather than answered with emptiness.

Only the node serving MCP needs the Observer application. Every node runs the built-in `system`
application that answers these questions, so one endpoint covers the cluster.

Observer combines capability ceilings for the deployment, listener and caller. A read-only ceiling omits mutation tools from the exposed catalog. Some diagnostic tools pause the observed runtime; the tool descriptions identify those costs so the client can limit collection.

For the surface in practice, see [Inspecting With an AI Agent](advanced/mcp.md); for configuring
and bounding it, [Observer](extra-library/applications/observer.md).

## Getting started

```
# Install the project generator
go install ergo.tools/ergo@latest

# Create a project
ergo init AgentNode github.com/myorg/agentnode
cd agentnode

# Add components
ergo add supervisor AgentNodeApp:AgentSup
ergo add actor AgentSup:ResearchAgent
ergo add actor AgentSup:AnalysisAgent

# Run
go run ./cmd
```

Add Observer to the generated node setup, which brings the MCP surface with it:

```go
import "ergo.services/application/observer"

options.Applications = []gen.ApplicationBehavior{
    agentnodeapp.CreateApp(),
    observer.CreateApp(observer.Options{Port: 9911}),
}
```

Connect your AI assistant and start investigating:

```
claude mcp add --transport http ergo http://localhost:9911/mcp
```

## Cloud-connected agents

The same actor addresses can be used across hosts when discovery and connection routes are reachable. Register message types on participating nodes, configure TLS and handle connection loss. [Network Stack](networking/network-stack.md) and [Behind the NAT](networking/behind-the-nat.md) explain those deployment conditions.

## Next steps

- [Process](basics/process.md) for the actor lifecycle
- [Supervisor](actors/supervisor.md) for restart strategies
- [Events](basics/events.md) for pub/sub coordination
- [Inspecting With an AI Agent](advanced/mcp.md) for AI-driven investigation of a live cluster
- [Observer](extra-library/applications/observer.md) for configuring and bounding that surface
- [Examples](https://github.com/ergo-services/examples) for working reference projects
