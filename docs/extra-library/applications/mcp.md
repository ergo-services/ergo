# MCP

The MCP application is a sidecar diagnostic tool for Ergo Framework nodes. It runs inside your node as a regular Ergo application and exposes 46 inspection tools via MCP (Model Context Protocol) over HTTP. AI agents like Claude Code connect to the endpoint and use these tools to diagnose performance bottlenecks, inspect processes, profile goroutines and heap, monitor metrics in real time, and trace issues across a cluster -- all without restarting or redeploying the node.

The application has two deployment modes. An **entry point** node runs an HTTP listener that accepts MCP requests. An **agent** node has no HTTP listener but is fully accessible through the entry point via cluster proxy. This means a single HTTP endpoint gives access to every node in the cluster.

The real power comes from combining MCP with other context the AI agent already has. When the agent can see your source code, inspect a live cluster in real time, and query your log storage (OpenSearch, Loki, CloudWatch) -- it connects the dots that no single tool can. It reads the actor implementation, checks its runtime state via MCP, correlates with error logs, and pinpoints the root cause. Source code explains intent. MCP shows what actually happens. Logs show the history. Together they eliminate guesswork.

## Why MCP

Traditional monitoring tools (Prometheus, Grafana) collect predefined metrics at fixed intervals and display them on dashboards. You decide upfront what to track, configure scraping, build dashboards, and then interpret the data yourself when something goes wrong.

MCP takes a different approach. Instead of predefined metrics, it exposes the full diagnostic API of the node -- 46 tools covering processes, applications, events, network, runtime, and profiling. An AI agent decides what to inspect based on the symptom you describe. It runs diagnostic sequences, correlates findings across tools, and narrows down root causes interactively.

This is particularly useful for:

- **Source code + live cluster + logs** -- the agent reads your actor code to understand what a process should do, inspects it via MCP to see what it actually does, and checks logs to see what happened before. This combination is far more powerful than any of these tools in isolation.
- **Ad-hoc investigation** -- you don't need to have anticipated the problem. The agent explores the node's state dynamically.
- **Distributed tracing** -- the cluster proxy lets the agent inspect any node from a single entry point, following issues across node boundaries.
- **Real-time sampling** -- active samplers periodically call any tool and store results in a ring buffer. Passive samplers capture log streams and event publications as they happen. The agent reads collected data incrementally.
- **Deep profiling** -- goroutine stack traces per process PID (with `-tags=pprof`), heap profiling, runtime stats, all accessible through tool calls.

MCP complements rather than replaces traditional monitoring. Use Prometheus/Grafana for long-term trends and alerting, and MCP for interactive investigation when alerts fire.

## Adding to Your Node

```go
import (
	"ergo.services/ergo"
	"ergo.services/application/mcp"
	"ergo.services/ergo/gen"
)

func main() {
	opt := gen.NodeOptions{
		Applications: []gen.ApplicationBehavior{
			mcp.CreateApp(mcp.Options{
				Port: 9922,
			}),
		},
	}
	node, err := ergo.StartNode("example@localhost", opt)
	if err != nil {
		panic(err)
	}
	node.Wait()
}
```

The function `mcp.CreateApp` takes `mcp.Options` as an argument:

* **Port**: The port number for the HTTP endpoint (default: `0` which means agent mode -- no HTTP listener).
* **Host**: The interface name (default: `localhost`).
* **Token**: Bearer token for authentication. Empty string disables authentication.
* **ReadOnly**: When `true`, disables action tools (`send_message`, `call_process`, `send_exit`, `process_kill`). Useful for production nodes where you want inspection without the ability to modify state.
* **AllowedTools**: A whitelist of tool names. When set, only the listed tools are available. `nil` or empty means all tools are enabled (respecting `ReadOnly`).
* **PoolSize**: The number of worker processes that handle incoming requests (default: `5`).
* **LogLevel**: The logging level for the MCP application processes.

## Connecting a Client

### Claude Code

```bash
# Add the MCP server
claude mcp add --transport http ergo http://localhost:9922/mcp

# With authentication
claude mcp add --transport http ergo http://localhost:9922/mcp \
  -H "Authorization: Bearer my-secret-token"
```

To allow all MCP tools without per-call permission prompts, add to `.claude/settings.json`:

```json
{
  "permissions": {
    "allow": [
      "mcp__ergo"
    ]
  }
}
```

The prefix `mcp__ergo` matches the server name from the `claude mcp add` command. Once configured, the agent can call any of the 46 tools directly.

### Other MCP Clients

The application implements MCP protocol version `2025-06-18` over HTTP. Any MCP-compatible client can connect by sending JSON-RPC 2.0 POST requests to `http://<host>:<port>/mcp`.

## What You Can Do

The 46 tools are organized into categories. You don't need to learn them -- the agent discovers available tools automatically via the MCP protocol. But understanding the categories helps you know what kinds of questions you can ask.

Every tool accepts an optional `node` parameter for cluster proxy. When specified and the target is a different node, the request is forwarded via native Ergo inter-node protocol.

### Inspect node and processes

The most common starting point. Ask the agent to show you what's happening on the node -- process counts, memory, CPU, running applications. Then drill into individual processes: who has the deepest mailbox, who consumes the most CPU, who was recently spawned.

Example prompts:
- "Show me the overall health of the node"
- "Which processes have the deepest mailboxes right now?"
- "Find processes that were spawned in the last 30 seconds"
- "Show me the supervision tree under the order_sup process"
- "What's the state of process worker_3?"

The `process_list` tool supports filtering (by application, behavior, state, name, numeric thresholds) and sorting (by mailbox depth, latency, running time, wakeups, drain ratio, etc.). This makes it the primary instrument for finding problematic processes.

### Monitor applications and events

Check application lifecycle (loaded, running, stopped), inspect dependencies, and diagnose pub/sub event issues. The event tools detect common problems: events publishing to nobody, subscribers waiting for data that never comes, fanout overload.

Example prompts:
- "Which applications are running? Any stopped?"
- "Show me events that have no subscribers"
- "Which events generate the most inter-node traffic?"
- "List all events owned by the order_handler process"

### Diagnose network and cluster

Inspect cluster connectivity, traffic between nodes, registrar state, and routes. Connect or disconnect nodes. The cluster proxy means you can inspect any node from a single entry point.

Example prompts:
- "Show me all nodes in the cluster and their status"
- "Is node backend@host connected? What's the traffic like?"
- "Connect to node worker@host and show its process list"
- "Check if the payment-service application is deployed across the cluster"

### Profile and debug

Deep diagnostics: goroutine stack traces (per-process with `-tags=pprof`), heap profiling, runtime stats. Find deadlocks, goroutine leaks, memory pressure.

Example prompts:
- "Show me the goroutine dump, group by stack"
- "Get the stack trace of process order_handler" (requires `-tags=pprof`)
- "What's the heap profile? Who allocates the most?"
- "Are there any processes stuck in WaitResponse?"

When a process is sleeping, its goroutine is parked and won't appear in the dump. The agent can use a sampler to poll until the process wakes up.

### Sample metrics over time

Snapshots are useful but trends tell the real story. Active samplers periodically call any tool and store results in a ring buffer. Passive samplers capture log messages and event publications as they happen.

Example prompts:
- "Monitor the top 10 mailbox offenders for 5 minutes"
- "Track memory and GC stats every 5 seconds"
- "Capture error and panic logs for the next 2 minutes"
- "Subscribe to the order_events event and show me what gets published"
- "Try to catch the goroutine of process worker_5 -- it's usually sleeping"

Active samplers are generic -- any tool can be sampled with any arguments. The `max_errors=0` parameter makes the sampler ignore errors and keep retrying, which is useful for polling rare conditions (like catching a sleeping process goroutine). All samplers are time-limited (default 60 seconds, maximum 1 hour).

### Manage log levels

Change log verbosity at runtime without restarting. Target the entire node, a specific process, or a meta process.

Example prompts:
- "Set debug logging on the payment_handler process"
- "What's the current log level for the node?"
- "List all registered loggers"

### Send messages and interact (action tools)

When `ReadOnly` is not set, the agent can send messages to processes, make synchronous calls with typed payloads from the EDF registry, send exit signals, and kill processes. These tools require explicit user permission.

Example prompts:
- "Send a StatusRequest to the worker_1 process and show the response"
- "What message types are registered in EDF?"
- "Gracefully stop the stuck_process with a shutdown signal"

The agent uses EDF type registry to construct typed Go structs from JSON. For example, if your application registers a `StatusRequest` type:

```go
// In your application code
type StatusRequest struct {
    Verbose bool
}

func init() {
    edf.RegisterTypeOf(StatusRequest{})
}
```

The agent can discover it with `message_types`, inspect its fields with `message_type_info`, and send it:

```
> Send a StatusRequest with Verbose=true to worker_1

Agent calls: message_type_info(type_name="StatusRequest")
  -> Fields: Verbose (bool)

Agent calls: call_process(to="worker_1", type_name="StatusRequest", request={"Verbose": true})
  -> Response: {"Status": "running", "Uptime": 3600}
```

The process receives a real `StatusRequest{Verbose: true}` Go struct in its `HandleCall` -- not raw JSON. The EDF registry handles the reflection and construction.

## Cluster Proxy

Every tool accepts a `node` parameter. When specified and the target differs from the local node, the request is forwarded to the remote node's MCP pool via native Ergo inter-node protocol -- not HTTP. The remote node must have the MCP application running, but agent mode (no HTTP) is sufficient.

This means you deploy one entry point node with HTTP and configure all other nodes as agents:

```go// Entry point node -- has HTTP listener
mcp.CreateApp(mcp.Options{Port: 9922})

// Agent nodes -- no HTTP, accessible via cluster proxy
mcp.CreateApp(mcp.Options{})
```

The agent connects to `http://entry-point:9922/mcp` and reaches any node in the cluster through the entry point. Connection between nodes happens through the standard Ergo network layer (registrar, static routes, or explicit connect).

## Build Tags

Two build tags enable additional diagnostic capabilities:

**`-tags=pprof`** enables per-process goroutine stack traces. When built with this tag, actor goroutines are labeled with their process PID via `runtime/pprof` labels. The `pprof_goroutines` tool with `pid` parameter extracts the stack trace of a specific actor's goroutine. Without this tag, the `pid` parameter returns an error.

**`-tags=latency`** enables mailbox latency measurement. The `process_list` tool gains `min_mailbox_latency_ms` filter and `mailbox_latency` sort field. Each process reports `MailboxLatency` -- how long the oldest message has been waiting in the mailbox. Without this tag, latency fields return -1.

Both tags add a small amount of overhead. Enable them in staging and production builds where diagnostics matter.

## Relationship to Metrics Actor

The [Metrics](../actors/metrics.md) actor collects predefined metrics into Prometheus format for scraping by external monitoring systems. The MCP application provides the same underlying data (it reads from `ProcessRangeShortInfo`, `NodeInfo`, `EventRangeInfo` -- the same sources) but exposes it interactively through tool calls.

Active samplers can replicate what the metrics actor does -- `sample_start tool=process_list arguments={"sort_by":"mailbox","limit":10}` is equivalent to the `ergo_mailbox_depth_top` Prometheus metric. The key difference is that MCP samplers are on-demand and agent-driven, while Prometheus metrics are always-on and scraper-driven.

Use the metrics actor when you need long-term time-series storage, alerting rules, and Grafana dashboards. Use MCP when you need interactive investigation, ad-hoc queries, and AI-assisted diagnostics.

## Agent and Skill for Claude Code

A ready-to-use Claude Code agent and skill are available at [github.com/ergo-services/claude](https://github.com/ergo-services/claude):

* **ergo-devops agent** (`claude/agents/ergo-devops.md`) -- interactive diagnostics agent that connects to a running node via MCP and runs diagnostic sequences. Contains playbooks for performance bottlenecks, process leaks, restart loops, zombie processes, memory growth, network issues, event system problems, goroutine investigation, and cluster health checks. Trigger it by describing a symptom ("why is it slow", "check the cluster", "find process leak").

* **ergo-devops skill** (`claude/skills/ergo-devops/SKILL.md`) -- compact reference with the same playbooks in recipe format, pattern matching tables, and sampler quick reference. Load it via `/ergo-devops` in Claude Code for quick lookup without starting a full investigation.

Install by symlinking into `~/.claude/`:

```bash
cd ergo.services/claude
ln -sf $(pwd)/agents/ergo-devops.md ~/.claude/agents/
ln -sf $(pwd)/skills/ergo-devops ~/.claude/skills/
```
