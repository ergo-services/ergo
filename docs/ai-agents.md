---
description: Build, run, and diagnose multi-agent AI systems on Ergo Framework
---

# AI Agents

Modern AI systems are multi-agent by nature. A research agent delegates to an analysis agent. A planner coordinates with executors. A conversation manager spawns short-lived task agents. Moving from a demo with a handful of agents to a production deployment with hundreds or thousands surfaces the same problems that distributed systems have solved for decades: crash isolation, supervision, cross-node coordination, observability at scale.

Ergo was built for telecom workloads where these requirements are baseline. AI agents have the same profile: many concurrent isolated workers with fault tolerance, coordination, and real-time behavior. This page shows how to use Ergo as runtime for your agents and as a live diagnostic surface for the running system.

## Why Ergo fits AI agents

Four problems appear as soon as you move AI agents out of a notebook:

**Agent crashes.** One stuck LLM call or panicking tool handler takes down the whole process. Everything running in that process dies with it.

**Coordination.** Agents need to talk to each other. Without a framework this becomes a web of channels, shared state, and custom routing code.

**Observability.** You can't see what's happening inside a running agent system. Mailbox depth, per-agent CPU, which agents are waiting on which external calls, where cascade failures originate.

**Scaling.** Distributing agents across nodes requires rethinking addressing, message delivery, and failure semantics.

Ergo addresses all four: isolated processes with supervision, named event streams for coordination, a built-in MCP diagnostic surface, and network-transparent PIDs. The design choices were made for telecom-class distributed systems. The fit to AI workloads is incidental and exact.

## Your agent as an actor

An AI agent in Ergo is just an actor: a process with private state and a mailbox, handling messages sequentially.

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
        result := callLLM(m.Query) // blocking call, isolated per agent
        a.notes = append(a.notes, result)
        a.Send(m.ReplyTo, result)
    }
    return nil
}

func factory_ResearchAgent() gen.ProcessBehavior { return &ResearchAgent{} }
```

What you get automatically:

- **Crash isolation.** A panicking LLM call or tool handler terminates only this actor. See [Process](basics/process.md).
- **Supervision.** Put the agent under a supervisor and it restarts on failure with your chosen strategy. See [Supervisor](actors/supervisor.md).
- **Distributed addressability.** Each agent has a PID that works across nodes. See [Remote Spawn Process](networking/remote-spawn-process.md).
- **Event-based coordination.** Agents publish to and subscribe to named event streams, fanning out one network message per node instead of one per subscriber. See [Events](basics/events.md).
- **Live diagnostics.** Expose the running system to any AI assistant through the MCP surface of the [Observer](extra-library/applications/observer.md) application.

The actor's private state (`notes` in the example) is safe without any synchronization. Messages arrive one at a time. The actor never shares memory with anyone.

## Multi-agent architecture patterns

### Agent Pool

Run N identical worker agents and distribute incoming tasks across them. Ideal for stateless agents that process requests in parallel (LLM calls, embedding lookups, tool invocations).

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
node.Send(poolPID, MessageResearchTask{Query: "Summarize Q3 report"})
```

Pool size and worker mailbox size together form a natural rate limit: at most `PoolSize × WorkerMailboxSize` tasks in flight. See [Pool](actors/pool.md).

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

Chain agents by sending from one stage to the next. Each stage runs under a supervisor. Failure in any stage is isolated and restarted.

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

If `AnalysisAgent` crashes, the supervisor restarts it without affecting the other stages. Pipelines compose naturally with pools: a stage can be a single actor or a pool of identical workers.

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

The framework delivers one network message per subscriber node regardless of how many subscribers that node has. 1M subscribers across 10 nodes cost 10 network messages, not 1M. See [Events](basics/events.md) and [Pub/Sub Internals](advanced/pub-sub-internals.md).

## Live diagnostics for AI systems

AI agents are nondeterministic. Behavior depends on prompts, external API latency, model temperature, and tool responses. Predefined metrics cover known failure modes, but the interesting failures are the ones you didn't anticipate.

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

Now you describe a symptom in plain English and the AI runs a diagnostic sequence against the live system:

```
You: "Why is the order processing agent slow?"

AI: Checking process list sorted by mailbox...
    -> order_processor has 847 queued messages (normal: <10)
    Inspecting order_processor upstream dependencies...
    -> payment_validator is processing 1 message per 3.2 seconds
    Checking payment_validator CPU profile...
    -> 73% time in external_api.Call(). The payment API is the bottleneck.
```

The surface has two halves. Readings are resources the agent reads by URI, `ergo://<node>/<lens>`,
and read again with a cursor to take only what has landed since. Everything else is a tool: 38 of
them, from a process listing to a heap profile, plus the two that put one question to many nodes at
once. Only the node serving MCP needs the Observer application: every node runs the built-in
`system` application it asks, so the whole cluster is reachable from one endpoint.

For the surface in full, what it costs the node, and how a ceiling bounds what an agent may do,
see [Observer](extra-library/applications/observer.md).

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

Running agents across AWS, GCP, Azure, or bare metal is supported via [ergo.cloud](https://ergo.cloud), a managed overlay network that connects nodes without VPNs, proxies, or tunnels. End-to-end encrypted. Currently available via waitlist.

## Next steps

- [Process](basics/process.md) for the actor lifecycle
- [Supervisor](actors/supervisor.md) for restart strategies
- [Events](basics/events.md) for pub/sub coordination
- [Observer](extra-library/applications/observer.md) for live diagnostics and AI-driven investigation
- [Examples](https://github.com/ergo-services/examples) for working reference projects
