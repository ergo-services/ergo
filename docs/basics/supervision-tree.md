---
description: Process control and fault tolerance
---

# Supervision Tree

Building reliable systems means accepting an uncomfortable truth: failures will happen. Hardware fails. Networks partition. Bugs exist in code. The question isn't whether your processes will crash, but what happens when they do.

The supervision tree model provides an answer. Instead of trying to prevent all failures, you structure your system so failures are expected, isolated, and automatically recovered from.

## The Supervision Principle

The model divides processes into two distinct roles:

**Workers** do the actual work. They handle requests, process data, manage state, and inevitably, sometimes crash when things go wrong.

**Supervisors** watch over workers. Their only job is to start child processes and restart them when they fail. Supervisors don't do application work - they manage lifecycle.

This separation is crucial. If workers handled their own restart logic, a bug in that logic would prevent recovery. By moving restart responsibility to a separate supervisor, you ensure that failures in workers can always be recovered.

## How Supervision Works

A supervisor starts its children and monitors them. When a child crashes, the supervisor decides what to do based on its restart strategy. Should it restart just this one child? Restart all children? Restart all children in a specific order?

The strategy depends on the relationships between children. If they're independent, restart just the failed one. If they depend on each other, restart all of them to ensure consistent state. If they have startup dependencies, restart in order.

Supervisors can supervise other supervisors, forming a tree. At the top might be an application supervisor. Below it, supervisors for different subsystems. Below those, the actual workers. When a worker crashes, only its portion of the tree is affected. The rest of the system continues running.

## Fault Tolerance Through Isolation

This tree structure creates fault isolation boundaries. A crashed database worker doesn't affect the HTTP handler workers. A failed cache process doesn't take down the authentication processes. Each supervision subtree handles its own failures without cascading them upward.

The Erlang community calls this "let it crash." It sounds reckless, but it's actually disciplined. Instead of defensive programming trying to handle every possible error, you let processes fail and rely on supervisors to restart them in a clean state. Often, a fresh restart clears transient problems that would be difficult to handle explicitly.

## Supervision in Ergo Framework

Ergo Framework implements supervision through the `act.Supervisor` actor. When you create a supervisor, you specify its children and restart strategy. The framework handles the monitoring and restart logic.

Workers are typically `act.Actor` implementations - regular actors that do application work. Supervisors are `act.Supervisor` implementations - actors whose behavior is managing children.

Because supervisors are also actors, they can be supervised. This is how you build the tree: supervisors supervising supervisors supervising workers, all the way down.

The tree structure emerges from how you compose supervisors and workers. There's no special tree-building API. You just nest supervisors, and the tree forms naturally.

## Building Reliable Systems

The supervision tree model leads to systems with interesting properties.

**Self-healing** - Failures trigger automatic recovery. Most transient problems resolve themselves through restart.

**Graceful degradation** - When a subsystem fails, only that part stops working. The rest continues serving requests.

**Operational simplicity** - Instead of complex error handling throughout your code, you centralize recovery logic in supervisors.

The trade-off is that you need to design processes that can restart cleanly. State that must survive restarts needs to be externalized - in databases, in other processes, or rebuilt from messages. But this discipline leads to more robust designs anyway.

## Where to Go From Here

Understanding supervision requires seeing it in practice. The [Supervisor](../actors/supervisor.md) chapter covers the specifics: restart strategies, child specifications, and practical patterns for structuring your application.

The combination of the actor model (isolated processes, message passing) and supervision trees (automatic recovery) gives you the tools to build systems that handle failures gracefully. It's a different approach than traditional error handling, but one that scales well to distributed systems where failures are inevitable.
