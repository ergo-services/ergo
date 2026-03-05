---
description: AI-powered diagnostics for running Ergo nodes via Model Context Protocol
---

# MCP

Diagnosing a distributed actor system is hard. The problem isn't a lack of data -- it's knowing what to look for. A node has hundreds of processes, dozens of connections, thousands of events flowing between them. Something is slow, but where? A process is stuck, but why? Memory is growing, but what's holding it?

Traditional monitoring collects predefined metrics at fixed intervals. You decide upfront what matters, build dashboards, and then interpret the data when something breaks. This works for known failure modes. It doesn't work when the failure is something you haven't anticipated -- and in distributed systems, the interesting failures are always unanticipated.

MCP takes a different approach. Instead of predefined metrics, it exposes the full diagnostic surface of the node -- processes, applications, events, network, profiling, runtime -- as tools that an AI agent can call on demand. The agent decides what to inspect based on the symptom you describe. It runs diagnostic sequences, correlates findings across tools, narrows down root causes, and explains what it found. You describe the problem in words; the agent finds the answer in data.

The real power comes from combination. The agent can see your source code, inspect the live cluster via MCP, and query your log storage -- all in the same conversation. It reads the actor implementation to understand intent, checks runtime state to see what actually happens, and correlates with error logs to see the history. Together these eliminate guesswork in a way no single tool can.

The application runs as a regular Ergo sidecar. Add it to your node's application list, and every process, connection, and event becomes inspectable -- without restarting, redeploying, or attaching a debugger.

## Two Deployment Modes

MCP has two modes: entry point and agent.

An entry point node runs an HTTP listener that accepts MCP protocol requests. This is the node your AI client connects to. An agent node has no HTTP listener at all -- it's invisible from outside the cluster. But it runs the same diagnostic tools internally, and any entry point can reach it through cluster proxy.

In practice, you deploy one entry point and make everything else an agent:

```go
import (
    "ergo.services/ergo"
    "ergo.services/application/mcp"
    "ergo.services/ergo/gen"
)

func main() {
    node, _ := ergo.StartNode("example@localhost", gen.NodeOptions{
        Applications: []gen.ApplicationBehavior{
            // Entry point -- the one HTTP endpoint for the entire cluster
            mcp.CreateApp(mcp.Options{Port: 9922}),
        },
    })
    node.Wait()
}
```

On every other node, the same application with no port:

```go
// Agent mode -- no HTTP, but fully diagnosable via cluster proxy
mcp.CreateApp(mcp.Options{})
```

The AI client connects to `http://entry-point:9922/mcp` and reaches any node in the cluster through that single endpoint.

## Configuration

```go
mcp.Options{
    Host:         "localhost",  // Listen address
    Port:         9922,         // HTTP port (0 = agent mode)
    Token:        "secret",     // Bearer token (empty = no auth)
    ReadOnly:     false,        // Disable action tools
    AllowedTools: nil,          // Tool whitelist (nil = all)
    PoolSize:     5,            // Worker processes
    CertManager:  nil,          // TLS certificate manager
    LogLevel:     gen.LogLevelInfo,
}
```

**Port** controls the deployment mode. A non-zero value starts an HTTP listener -- this is an entry point. Zero means agent mode: no listener, accessible only via cluster proxy from another node that has an entry point.

**Token** enables Bearer token authentication. When set, every HTTP request must include `Authorization: Bearer <token>`. When empty, no authentication is required. Agent mode nodes don't need a token -- they're accessed through the Ergo inter-node protocol, which has its own authentication via handshake cookies.

**ReadOnly** disables tools that modify state: `send_message`, `call_process`, `send_exit`, `process_kill`. Everything else -- inspection, profiling, sampling -- remains available. Use this on production nodes where you want full visibility without the ability to interfere.

**AllowedTools** restricts the tool set to a whitelist. When set, only the named tools are available. This is finer-grained than ReadOnly -- you can, for example, allow `send_message` but not `process_kill`. When nil, all tools are enabled (respecting ReadOnly).

## Connecting a Client

### Claude Code

```bash
claude mcp add --transport http ergo http://localhost:9922/mcp

# Available from any directory (user scope)
claude mcp add --transport http ergo --scope user http://localhost:9922/mcp

# With authentication
claude mcp add --transport http ergo http://localhost:9922/mcp \
  -H "Authorization: Bearer my-secret-token"
```

To allow all MCP tools without per-call permission prompts, add to `.claude/settings.json`:

```json
{
  "permissions": {
    "allow": ["mcp__ergo"]
  }
}
```

### Other Clients

The application implements MCP protocol version `2025-06-18` over HTTP. Any MCP-compatible client can connect by sending JSON-RPC 2.0 POST requests to `http://<host>:<port>/mcp`.

## How Cluster Proxy Works

Every tool accepts a `node` parameter. When specified, the entry point node forwards the request to the target node via native Ergo inter-node protocol -- not HTTP. The target node's MCP worker executes the tool locally and returns the result through the same path.

This works because of network transparency. The entry point calls `gen.ProcessID{Name: "mcp", Node: targetNode}` -- the framework establishes a connection if needed, routes the request, and delivers the response. You never need to explicitly connect to a node before querying it. If the registrar knows about the target node, the connection happens automatically.

The `timeout` parameter (default 30 seconds, max 120) controls how long the entry point waits for a remote response. Most tools respond in milliseconds. But CPU profiling collects data for a requested duration before responding, and goroutine dumps on large nodes take time to serialize. For these, pass a higher timeout.

If a remote tool call fails with "remote call failed", it usually means the target node doesn't have the MCP application running. All proxy calls require an MCP pool process on the target node -- agent mode is sufficient, but the application must be loaded and started.

## Profiling Remote Nodes

Profiling tools generate large output. A goroutine dump from a node with 500 goroutines can be megabytes of text. A heap profile with hundreds of allocation sites isn't much smaller. Push all of that through the proxy chain -- remote node, entry point, HTTP, JSON-RPC -- and you hit timeouts or transport limits.

The solution is server-side filtering. All profiling tools accept `filter` and `exclude` parameters that reduce the output before it leaves the remote node. Instead of transferring 500 goroutine stacks and searching locally, you tell the remote node to return only the stacks that match:

```
pprof_goroutines node=backend@host debug=1 filter="orderHandler" limit=20
```

The response header preserves the full picture: `goroutine profile: total 500, matched 3, showing 3`. You know the node has 500 goroutines, but only 3 matched your filter, and all 3 were returned. The agent can refine the filter, broaden it, or switch to a different angle -- each query is cheap because the heavy lifting happens on the remote node.

### CPU Profiling

The `pprof_cpu` tool collects a CPU profile for a given duration and returns the top functions by CPU usage:

```
pprof_cpu node=backend@host duration=5 exclude="runtime" limit=15 timeout=30
```

The node samples CPU activity for 5 seconds, aggregates by function, filters out Go runtime internals, and returns the top 15 application functions with flat and cumulative percentages. The `timeout` should be higher than `duration` to account for collection and transfer time.

### Heap Profiling

The `pprof_heap` tool shows the top memory allocators with two columns: `inuse` (live objects currently in memory) and `alloc` (cumulative allocations over the node's lifetime). A function with low `inuse` but high `alloc` is churning memory -- allocating and releasing rapidly, putting pressure on the garbage collector.

```
pprof_heap node=backend@host filter="myapp" limit=20
```

### Goroutine Analysis

The `pprof_goroutines` tool has two modes. Without `pid`, it returns all goroutines on the node -- use `filter` and `exclude` to narrow down. With `pid`, it returns the stack trace of a specific process's goroutine (requires `-tags=pprof`).

Debug level controls the output format: `debug=1` groups goroutines by identical stack (compact summary with counts), `debug=2` shows individual goroutine traces with state and wait duration.

A sleeping process parks its goroutine -- it won't appear in the dump. To catch it, use an active sampler that polls until the process wakes up:

```
sample_start tool=pprof_goroutines arguments={"pid":"<ABC.0.1005>"} interval_ms=300 count=1 max_errors=0
```

The sampler ignores the "goroutine not found" error (`max_errors=0`) and keeps polling every 300ms until it catches the process in a non-sleep state.

## Samplers

Snapshots show one moment. Trends show the story. Samplers bridge this gap by collecting data into ring buffers that the agent reads incrementally.

### Active Samplers

An active sampler periodically calls any MCP tool and stores the results. It's a generic periodic executor -- any tool with any arguments can be sampled.

```
sample_start tool=process_list arguments={"sort_by":"mailbox","limit":10} interval_ms=5000 duration_sec=300
```

This calls `process_list` every 5 seconds for 5 minutes, storing each result in a ring buffer. The agent reads with `sample_read sampler_id=<id>` to get all buffered entries, or `sample_read sampler_id=<id> since=5` to get only entries newer than sequence 5.

The `max_errors` parameter controls error tolerance. The default (0) means ignore all errors and keep retrying -- useful for polling rare conditions. A non-zero value stops the sampler after that many consecutive failures.

### Passive Samplers

A passive sampler listens for events instead of polling. It captures log messages and event publications as they happen:

```
sample_listen log_levels=["warning","error"] duration_sec=120
sample_listen event=order_events duration_sec=60
sample_listen log_levels=["error"] event=order_events duration_sec=120
```

Log capture and event subscription can be combined in a single sampler.

### Linger

Every sampler has a `linger_sec` parameter (default 30). After the sampler completes -- duration expires, count reached, or max errors exceeded -- it stays alive for this many additional seconds so the agent can retrieve the collected data. Without linger, a sampler that runs for 10 seconds would terminate before the agent gets a chance to read the results.

The `sample_list` tool shows sampler status: `running`, `completed, lingering 25s`, or `completed`. The `sample_stop` tool terminates a sampler immediately, bypassing the linger period.

### What to Sample

| Goal | Sampler |
|------|---------|
| Mailbox pressure trend | `sample_start tool=process_list arguments={"sort_by":"mailbox","limit":10}` |
| Memory and GC trend | `sample_start tool=runtime_stats interval_ms=5000` |
| Error storm detection | `sample_listen log_levels=["error","panic"]` |
| Event traffic monitoring | `sample_listen event=<name>` |
| Network health trend | `sample_start tool=network_nodes interval_ms=30000` |
| CPU hotspot sampling | `sample_start tool=pprof_goroutines arguments={"debug":1,"filter":"ProcessRun","exclude":"toolPprof","limit":20} interval_ms=500` |

## Typed Messages

When `ReadOnly` is not set, the agent can send messages to processes and make synchronous calls using the EDF type registry. This isn't raw JSON injection -- the framework constructs real Go structs from the type information.

If your application registers a type:

```go
type StatusRequest struct {
    Verbose bool
}

func init() {
    edf.RegisterTypeOf(StatusRequest{})
}
```

The agent discovers it with `message_types`, inspects its fields with `message_type_info`, and sends it with `call_process`. The process receives a real `StatusRequest{Verbose: true}` in its `HandleCall` -- not a map or raw bytes.

This makes interactive debugging possible: the agent can call any process with any registered request type, inspect the response, and reason about the behavior.

## Network Diagnostics

The `network_ping` tool sends a request through the full network path -- flusher, TCP connection, remote MCP worker, response -- and measures the round-trip time. This is an end-to-end health check, not a TCP-level ping. If the flusher is broken, the connection pool is degraded, or the remote node is overloaded, the ping will reflect it.

```
network_ping name=backend@host
→ ping backend@host: rtt 0.42ms
```

For deeper connection analysis, `network_node_info` shows per-connection statistics: messages in/out, bytes in/out, pool size, pool DSN (which side dialed), and a `Reconnections` counter that tracks how many times pool items have reconnected. A non-zero reconnection count indicates connection instability.

When investigating connection problems, always check both sides:

```
network_node_info node=A name=B    -- A's view of the connection to B
network_node_info node=B name=A    -- B's view of the connection to A
```

Asymmetry between the two sides -- one sees thousands of messages out while the other sees one message in -- indicates data loss at the connection level.

## Build Tags

Two build tags enable additional diagnostic capabilities. Both add a small amount of overhead and should be enabled in staging and production builds where diagnostics matter.

**`-tags=pprof`** enables the Go profiler and labels actor goroutines with their process PID. The labels appear in goroutine dumps as `{"pid":"<ABC123.0.1005>"}` for actors and `{"meta":"Alias#...", "role":"reader"}` for meta processes. The `pprof_goroutines` tool with `pid` parameter uses these labels to extract a specific actor's stack trace. Without this tag, the `pid` parameter returns an error.

This tag also starts a pprof HTTP endpoint at `localhost:9009/debug/pprof/` (configurable via `PPROF_HOST` and `PPROF_PORT` environment variables) for use with `go tool pprof`.

**`-tags=latency`** enables mailbox latency measurement. Each mailbox queue tracks the age of its oldest unprocessed message. The `process_list` tool gains `min_mailbox_latency_ms` filter and `mailbox_latency` sort field. Without this tag, latency fields return -1.

## Relationship to Metrics Actor

The [Metrics](../actors/metrics.md) actor collects predefined metrics into Prometheus format for scraping. MCP reads from the same underlying data sources -- `ProcessRangeShortInfo`, `NodeInfo`, `EventRangeInfo` -- but exposes them interactively.

Active samplers can replicate any Prometheus metric: `sample_start tool=process_list arguments={"sort_by":"mailbox","limit":10}` is equivalent to `ergo_mailbox_depth_top`. The difference is that MCP samplers are on-demand and agent-driven, while Prometheus metrics are always-on and scraper-driven.

Use the metrics actor for long-term trends, alerting, and Grafana dashboards. Use MCP for interactive investigation when alerts fire or when you need to explore something unexpected.

## Agent and Skill for Claude Code

A ready-to-use diagnostic agent and skill are available at [github.com/ergo-services/claude](https://github.com/ergo-services/claude). The agent contains playbooks for common scenarios: performance bottlenecks, process leaks, restart loops, zombie processes, memory growth, network issues, event system problems, goroutine investigation, and cluster health checks. Trigger it by describing a symptom -- "why is it slow", "check the cluster", "find the process leak" -- and it runs the appropriate diagnostic sequence.

Install by symlinking into `~/.claude/`:

```bash
cd ergo.services/claude
ln -sf $(pwd)/agents/ergo-devops.md ~/.claude/agents/
ln -sf $(pwd)/skills/ergo-devops ~/.claude/skills/
```

## Full Tool Reference

The complete list of 48 tools with parameters and descriptions is in the [MCP application README](https://github.com/ergo-services/application/blob/master/mcp/README.md).
