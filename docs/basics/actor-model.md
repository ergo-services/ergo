---
description: The Actor Model and Its Properties
---

# Actor Model

The actor model is a computational approach to building concurrent systems, first proposed in the 1970s. Instead of sharing memory and coordinating through locks, components communicate by sending messages to each other.

An actor keeps state and responds to messages. For example, a cache actor owns a map: callers ask it to store or retrieve a key. It processes those requests sequentially, so accesses to the map do not overlap.

## What Makes an Actor

An actor consists of three things:

**Private State** - Data owned by the actor. In Go, ownership depends on keeping mutable references inside the actor or transferring them without retaining access.

**Behavior** - The logic that determines how the actor responds to messages. This can change over time as the actor processes different messages.

**Mailbox** - A queue where incoming messages wait to be processed. The actor pulls messages from this queue one at a time.

When an actor receives a message, it can do three things: send messages to other actors, create new actors, or decide how to handle the next message.

## Sequential Processing

Each actor processes messages sequentially, one after another.

In a lock-based design several threads reach the same data, and the locks that keep it consistent bring their own problems: deadlocks, lock ordering, and reasoning about what the data holds at any given moment.

Since only one message is processed at a time, state that the actor alone reaches moves from one well-defined value to the next, with no interleaving.

## Location Transparency

An actor address identifies its destination node. The messaging API accepts that address whether the node is local or remote.

The same messaging API is used for local and remote processes, so code written for a single machine runs unchanged across several. Remote delivery keeps its own failure semantics: the connection can drop, and the peer can be gone before the message arrives.

## Real-World Implementations

**Erlang** pioneered the practical application of the actor model. The language and its BEAM virtual machine have been running telecommunications systems since the 1980s.

**Akka** brought the actor model to the Java ecosystem. It's used in systems that need to process high-volume transactions, manage complex workflows, or handle real-time data streams.

**Orleans** applies the actor model in cloud environments. Its virtual actor pattern creates and destroys actors on demand.

## How This Applies to Go

Go provides goroutines and channels, but leaves process identity, mailbox dispatch and lifecycle relationships to application code. Goroutines may share memory; using Ergo does not change that language rule.

Ergo assigns each process a mailbox and an address. It runs one handler at a time for that process, while different processes run concurrently.

Local messages pass Go values without copying or serializing their contents. If a value contains a map, slice or pointer, sender and receiver can access the same memory. Concurrent mutation may cause a data race. Messages crossing a node connection are encoded and decoded into separate values.

Keep mutable state private by sending independent values or transferring ownership and ending the sender's access. The [argus](../tools/argus.md) analyzer checks message types for shared-memory risks with rule A1001.

## Shared Data as an Actor

A typical pattern: instead of several goroutines sharing a cache behind a mutex, the cache is an actor. Reads and writes are messages, handled one at a time, so the map itself is reached from one goroutine only.

Partitioning the key space across several cache actors, placing them under a supervisor, or moving them to other nodes does not change how callers address them.

## Next

Continue with [Node](node.md) to start the runtime, then [Process](process.md) and [Actor](../actors/actor.md) to create a message-handling instance. Return to [Supervision Tree](supervision-tree.md) once process termination is familiar.
