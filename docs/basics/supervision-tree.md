---
description: Process control and fault tolerance
---

# Supervision Tree

Hardware fails, networks partition, and code has bugs. The question isn't whether your processes will crash, but what happens when they do.

Instead of trying to prevent all failures, you structure your system so failures are expected, isolated, and automatically recovered from.

## The Supervision Principle

The model divides processes into two distinct roles:

**Workers** do the actual work. They handle requests, process data, manage state, and inevitably, sometimes crash when things go wrong.

**Supervisors** watch over workers. Their only job is to start child processes and restart them when they fail. Supervisors don't do application work - they manage lifecycle.

If workers handled their own restart logic, a bug in that logic would prevent recovery. Restart responsibility is kept in a separate process for that reason.

## How Supervision Works

A supervisor starts its children and monitors them. When a child crashes, the supervisor decides what to do based on its restart strategy. Should it restart just this one child? Restart all children? Restart all children in a specific order?

The strategy depends on the relationships between children. If they're independent, restart just the failed one. If they depend on each other, restart all of them to ensure consistent state. If they have startup dependencies, restart in order.

Supervisors can supervise other supervisors, forming a tree. At the top might be an application supervisor. Below it, supervisors for different subsystems. Below those, the actual workers. When a worker crashes, only its portion of the tree is affected. The rest of the system continues running.

## Fault Tolerance Through Isolation

This tree structure creates fault isolation boundaries. A crashed database worker doesn't affect the HTTP handler workers. A failed cache process doesn't take down the authentication processes. Each supervision subtree handles its own failures without cascading them upward.

The Erlang community calls this "let it crash." Instead of defensive programming trying to handle every possible error, you let processes fail and rely on supervisors to restart them in a clean state. Often, a fresh restart clears transient problems that would be difficult to handle explicitly.

## Supervision in Ergo Framework

Ergo Framework implements supervision through the `act.Supervisor` actor. When you create a supervisor, you specify its children and restart strategy. The framework handles the monitoring and restart logic.

Workers are typically `act.Actor` implementations - regular actors that do application work. Supervisors are `act.Supervisor` implementations - actors whose behavior is managing children.

Because supervisors are also actors, they can be supervised. This is how you build the tree: supervisors supervising supervisors supervising workers, all the way down.

The tree structure emerges from how you compose supervisors and workers. There's no special tree-building API. You just nest supervisors, and the tree forms naturally.

## Building Reliable Systems

**Self-healing** - Failures trigger automatic recovery. Transient problems often resolve through restart.

**Graceful degradation** - When a subsystem fails, only that part stops working. The rest continues serving requests.

**Operational simplicity** - Instead of complex error handling throughout your code, you centralize recovery logic in supervisors.

The trade-off is that you need to design processes that can restart cleanly. State that must survive restarts needs to be externalized - in databases, in other processes, or rebuilt from messages.

## Where to Go From Here

Understanding supervision requires seeing it in practice. The [Supervisor](../actors/supervisor.md) chapter covers the specifics: restart strategies, child specifications, and practical patterns for structuring your application.
