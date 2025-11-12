---
description: Linking and Monitoring Mechanisms
---

# Links and Monitors

Building reliable systems from independent processes requires solving a fundamental coordination problem. When a process terminates - whether from a crash, graceful shutdown, or network failure - other processes that depend on it or supervise it need to know. Without this knowledge, a supervisor can't restart failed workers, dependent processes continue attempting to use unavailable services, and the system degrades silently.

The challenge is detecting termination without breaking isolation. Processes can't share memory or directly observe each other's state. The traditional approach in distributed systems uses heartbeats: processes periodically signal they're alive, and silence implies failure. But heartbeats introduce overhead, timing sensitivity, and the fundamental ambiguity of distinguishing "slow" from "dead."

Ergo Framework provides a different mechanism. Processes explicitly declare relationships - links and monitors - and the framework delivers termination notifications through these channels. When a process terminates, the node automatically notifies all processes that established relationships with it. The notification is immediate, deterministic, and part of the normal message flow.

Links and monitors both deliver termination notifications, but they differ in what happens next. A link couples your lifecycle to the target's - when it terminates, you terminate. A monitor simply informs you of termination, leaving the response up to you. The choice depends on whether you need failure propagation or just failure awareness.

## Links: Coupling Lifecycles

Creating a link to another process declares a dependency. You're stating that your operation depends on the target's continued existence. When the target terminates, you receive an exit signal - a high-priority message that typically causes your termination as well.

Exit signals arrive in the Urgent queue, bypassing normal message ordering. The default behavior is immediate termination when an exit signal arrives. This cascading failure makes sense in many scenarios. If a worker's connection to a critical service is gone, the worker has nothing useful to do and should terminate cleanly.

But sometimes you want to handle exit signals explicitly. Actors can enable exit signal trapping through `act.Actor`. When trapping is enabled, exit signals are delivered as `gen.MessageExit*` messages to your `HandleMessage` callback. You can examine the signal, check the termination reason, and decide whether to terminate or attempt recovery.

Each exit message type carries the termination reason in its `Reason` field. The reason tells you what happened: normal shutdown (`gen.TerminateReasonNormal`), abnormal crash, panic (`gen.TerminateReasonPanic`), forced kill (`gen.TerminateReasonKill`), or network failure (`gen.ErrNoConnection`). This context lets you make informed decisions about how to react.

The framework provides linking methods for different identification schemes. `LinkPID` takes a process identifier and links to that specific process instance. When it terminates, you receive `gen.MessageExitPID`. `LinkProcessID` links to a registered name rather than a specific instance. If the process terminates or unregisters the name, you receive `gen.MessageExitProcessID`. `LinkAlias` works with process aliases - termination or alias deletion triggers `gen.MessageExitAlias`.

You can also link to node connections with `LinkNode`. If the connection to the specified node is lost, you receive `gen.MessageExitNode`. This is useful for processes that can't operate when a particular remote node is unavailable.

The generic `Link` method accepts any target type and dispatches to the appropriate typed method. Use it when the target type varies, or use the specific methods when you know the type.

### The Unidirectional Nature

Links in Ergo are unidirectional, and this deserves emphasis because it differs from Erlang.

When you execute `process.LinkPID(target)`, you establish a relationship where target's termination affects you. The link points from you to the target. If the target terminates, you receive an exit signal. But if you terminate, the target is unaffected. The link doesn't point backward.

Erlang's links are bidirectional. If process A links to process B in Erlang, either terminating causes the other to terminate. This symmetry can be useful, but it also creates unexpected cascading failures. In Ergo, if you want bidirectional coupling, you create two links: A links to B, and B links to A.

The unidirectional design gives you precise control. Consider a shared service with multiple workers. Each worker links to the service (if the service dies, workers should too). But the service doesn't link back to workers (a worker crash shouldn't kill the service). Unidirectional links express this asymmetric dependency naturally.

## Monitors: Observation Without Coupling

Monitors provide lifecycle awareness without lifecycle coupling. You track when something terminates, but you don't terminate yourself.

The quintessential monitor use case is supervision. A supervisor monitors worker processes. When a worker terminates, the supervisor receives a down message. The message includes the worker's PID or identifier and the termination reason. The supervisor examines this information, consults its restart strategy, and decides whether to spawn a replacement. The supervisor continues running regardless of how many workers have crashed.

Down messages arrive in the System queue with high priority (but lower than Urgent exit signals). Each `gen.MessageDown*` type includes a `Reason` field. For `MonitorPID`, you receive `gen.MessageDownPID` with the target's PID and reason. For `MonitorProcessID`, you receive `gen.MessageDownProcessID` with the registered name and reason. The reason might indicate normal termination, a crash, or a special case like name unregistration (`gen.ErrUnregistered`).

Monitoring registered names or aliases handles invalidation gracefully. If you monitor a process by name and that process unregisters its name, you receive a down message with reason `gen.ErrUnregistered`. The process might still be running, but it's no longer accessible by that name, which is what you were monitoring. Same logic applies to alias deletion - you're notified that the thing you were monitoring is no longer valid.

Node monitoring tracks connection health. `MonitorNode` sends you `gen.MessageDownNode` when the connection to a remote node is lost. The reason is `gen.ErrNoConnection`. This is useful for detecting network partitions or remote node crashes without linking (which would terminate your process).

## Network Transparency in Practice

Links and monitors work across nodes without changing their semantics or your code.

When you link or monitor a remote target, the framework sends a request to the remote node. The remote node records that your process is watching the target. This setup happens during your `Link*` or `Monitor*` call and involves a network round-trip. The operation can fail if the remote node is unreachable or the target doesn't exist - check the error return.

Once established, the remote node tracks your subscription. When the target terminates on the remote node, the remote node sends a notification message back to your node. Your node routes it to your process's mailbox. From your perspective, it's just another message - you don't see the network mechanics.

Network failures complicate this. If the connection to the remote node fails while your link or monitor is active, your local node detects the disconnection. It looks up which local processes had links or monitors to targets on that failed node. For links, it sends exit signals with reason `gen.ErrNoConnection`. For monitors, it sends down messages with the same reason.

This unified handling means you write the same error handling code for local and remote targets. The notification mechanism is consistent. The reason field distinguishes between target termination and network failure, but the notification path is identical.

## Removing Links and Monitors

Links and monitors aren't permanent. You can remove them explicitly or they're removed automatically when participants terminate.

To remove a link, use the corresponding `Unlink*` method with the same target. `UnlinkPID`, `UnlinkProcessID`, `UnlinkAlias`, `UnlinkNode` each remove the link created by their `Link*` counterpart. If you never created the link, unlinking returns an error. For monitors, the `Demonitor*` methods work the same way.

When the target terminates and you receive notification, the link or monitor is automatically removed. You receive one notification per relationship. If the target is later restarted (by a supervisor), you won't receive notification about that new instance unless you create a new link or monitor to it.

When you terminate (the process that created the link or monitor), your relationships are cleaned up automatically. The target doesn't receive notification that you stopped watching. This asymmetry is intentional - the target doesn't track who's watching it, so it doesn't care when watchers go away.

## Practical Usage Patterns

Several common patterns emerge from combining links and monitors.

Workers often link to infrastructure processes they depend on. A worker processing HTTP requests might link to a database connection pool process. If the pool terminates (perhaps during a deployment), the worker receives an exit signal and terminates. The worker's supervisor detects the termination, waits a moment (hoping the database pool restarts), and spawns a new worker. The new worker links to the (now running) pool and resumes processing.

Supervisors monitor their children. Each worker termination triggers a down message. The supervisor checks the reason. If it's `gen.TerminateReasonNormal`, the worker finished its task and doesn't need restart. If it's an error or panic, the supervisor spawns a replacement. The supervisor's continued operation despite worker failures is the whole point of the supervisor pattern.

Load balancers monitor backend processes. Each backend termination updates the balancer's routing table. The balancer continues routing to available backends. When a backend restarts, it might need to register with the balancer, which would then monitor it again.

Parent-child relationships often use `LinkChild` and `LinkParent` options in `gen.ProcessOptions`. These create links during process initialization, working around the restriction that link methods aren't available during the Init state. If either participant terminates, the other receives an exit signal.

## The Difference That Matters

Links propagate failure. Monitors report failure. Choose based on whether the watcher should terminate when the target terminates.

If continued operation without the target is meaningless, use a link. If you can adapt to the target's absence (by finding a replacement, degrading gracefully, or restarting the target), use a monitor.

The unidirectional nature of links matters more than you might initially think. It lets you express asymmetric dependencies precisely. Workers depend on services, but services don't depend on individual workers. Clients depend on servers, but servers don't depend on individual clients. Links point from the dependent to the dependency, making the relationship clear.

For event-based publish/subscribe patterns using links and monitors, see the [Events](events.md) chapter. For supervision trees built on monitors, see [Supervisor](../actors/supervisor.md).
