---
description: Process control and fault tolerance
---

# Supervision Tree

A supervision tree records which processes are responsible for starting and replacing others. Workers perform the application work; supervisors decide how their children recover from termination.

A branch may contain independent workers or processes that must restart together. The examples below show how those dependencies determine the shape of the tree. The callback and policy details are in [Supervisor](../actors/supervisor.md).

## The Supervision Principle

The model divides processes into two distinct roles:

**Workers** do the actual work. They handle requests, process data, manage state, and inevitably, sometimes crash when things go wrong.

**Supervisors** watch over workers. Their only job is to start child processes and restart them when they fail. Supervisors don't do application work - they manage lifecycle.

If workers handled their own restart logic, a bug in that logic would prevent recovery. Restart responsibility is kept in a separate process for that reason.

## How Supervision Works

A supervisor starts its declared children and links to them. When a child terminates, the exit signal gives the supervisor a chance to apply its policy: restart that child, restart a related group, or let the child stay stopped.

The strategy depends on the relationships between children. If they're independent, restart just the failed one. If they depend on each other, restart all of them to ensure consistent state. If they have startup dependencies, restart in order.

Supervisors can also be children of other supervisors. A top-level supervisor might own an ingestion branch and an HTTP branch, with workers below each. A restart confined to the ingestion branch leaves HTTP processes running. If the ingestion supervisor exceeds its restart budget and terminates, its parent decides how far recovery extends.

## Fault Tolerance Through Isolation

Suppose an ingestion branch contains a socket reader and a decoder. If the reader owns no state needed by the decoder, restarting just the reader may be sufficient. If the decoder keeps framing state for that reader's stream, retaining it after the connection is replaced can combine bytes from different sessions. Restarting the related children together gives the new session fresh state.

An HTTP branch need not restart with ingestion if it does not depend on that session. Put that boundary into the tree and choose the parent policy accordingly. A tree does not infer dependencies from the application's code.

This is the reason to delegate recovery to a separate process. The worker can terminate when its state is no longer usable; the supervisor still has the specification needed to create a replacement. Restarting may clear a transient fault, but a bad configuration can make the replacement fail again. Restart intensity prevents that loop from continuing indefinitely.

## Supervision in Ergo Framework

Ergo Framework implements supervision through the `act.Supervisor` actor. When you create a supervisor, you specify its children and restart strategy. The framework handles the monitoring and restart logic.

Workers are typically `act.Actor` implementations - regular actors that do application work. Supervisors are `act.Supervisor` implementations - actors whose behavior is managing children.

Because supervisors are also actors, they can be supervised. This is how you build the tree: supervisors supervising supervisors supervising workers, all the way down.

The tree structure emerges from how you compose supervisors and workers. There's no special tree-building API. You just nest supervisors, and the tree forms naturally.

## Building Reliable Systems

The restart policy controls process lifecycles. It does not recover database transactions, a partially written external file or the message being handled when a callback panicked. Decide separately where durable state lives and how a replacement recognizes work already performed.

A supervisor policy also applies only to its own children. Suppose a supervised connection manager starts a reader itself. That reader is not a second child in the supervisor's declared group. If the reader and manager must stop together, the manager can establish both directions of the link when spawning:

```go
reader, err := manager.Spawn(factoryReader, gen.ProcessOptions{
    LinkChild:  true, // manager receives an exit when reader stops
    LinkParent: true, // reader receives an exit when manager stops
})
```

With ordinary `act.Actor` exit handling, reader termination stops the manager. The supervisor then observes its direct child, the manager, terminating and applies that child's restart policy. The manager's replacement creates a new reader during initialization. If the manager traps exits, it must explicitly decide how to handle the reader's signal.

For a dependency between two declared supervisor children, express it through the supervisor type instead. [Links and Monitors](links-and-monitors.md) covers link direction, and [Supervisor](../actors/supervisor.md) develops the restart strategies.

## Where to Go From Here

Understanding supervision requires seeing it in practice. The [Supervisor](../actors/supervisor.md) chapter covers the specifics: restart strategies, child specifications, and practical patterns for structuring your application.
