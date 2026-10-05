---
description: The Actor Model and Its Properties
---

# Actor Model

The actor model is a computational approach to building concurrent systems, first proposed in the 1970s. Instead of sharing memory and coordinating through locks, components communicate by sending messages to each other.

In the actor model, everything is an actor. An actor is an independent entity that has its own private state and processes incoming messages one at a time. Actors never directly access each other's state. Instead, they send messages and wait for responses if needed.

## What Makes an Actor

An actor consists of three things:

**Private State** - Data that belongs exclusively to this actor. No other actor can read or modify it directly.

**Behavior** - The logic that determines how the actor responds to messages. This can change over time as the actor processes different messages.

**Mailbox** - A queue where incoming messages wait to be processed. The actor pulls messages from this queue one at a time.

When an actor receives a message, it can do three things: send messages to other actors, create new actors, or decide how to handle the next message.

## Sequential Processing

Each actor processes messages sequentially, one after another.

In a lock-based design several threads reach the same data, and the locks that keep it consistent bring their own problems: deadlocks, lock ordering, and reasoning about what the data holds at any given moment.

Since only one message is processed at a time, state that the actor alone reaches moves from one well-defined value to the next, with no interleaving.

## Location Transparency

When you send a message to an actor, you don't need to know whether it's running in the same process, on the same machine, or halfway around the world. The addressing and the API are the same.

The same messaging API is used for local and remote processes, so code written for a single machine runs unchanged across several. Remote delivery keeps its own failure semantics: the connection can drop, and the peer can be gone before the message arrives.

## Real-World Implementations

**Erlang** pioneered the practical application of the actor model. The language and its BEAM virtual machine have been running telecommunications systems since the 1980s.

**Akka** brought the actor model to the Java ecosystem. It's used in systems that need to process high-volume transactions, manage complex workflows, or handle real-time data streams.

**Orleans** applies the actor model in cloud environments. Its virtual actor pattern creates and destroys actors on demand.

## How This Applies to Go

Go has goroutines and channels, which seem similar to actors and message passing. But there's a crucial difference: goroutines are not isolated. They can share memory, which means you still need locks and face the same concurrency challenges as traditional threading.

Ergo Framework brings actor model semantics to Go. Each process is an actor with its own goroutine and its own mailbox, and the only way in is a message. The framework never runs two handlers of the same process at the same time; different processes run concurrently.

One boundary the language cannot enforce for us, and it is worth knowing from the start: **memory isolation is a discipline the framework supports, not a constraint it imposes.** A message between two processes on the same node is handed over as the Go value it is, with no copy and no serialization. Send a map, a slice or a pointer and both processes then hold the same memory, and Go's race detector will say so. Only a message that crosses a node boundary is encoded, and the encoding is what makes the copy.

So "no shared state" is yours to keep: send values rather than references, or treat a send as handing over ownership and stop touching what you sent. The framework ships a vet tool, [argus](../tools/argus.md), whose A1001 rule flags exactly this.

## Shared Data as an Actor

A typical pattern: instead of several goroutines sharing a cache behind a mutex, the cache is an actor. Reads and writes are messages, handled one at a time, so the map itself is reached from one goroutine only.

Partitioning the key space across several cache actors, placing them under a supervisor, or moving them to other nodes does not change how callers address them.

## Next

The following chapters explore how these concepts manifest in Ergo Framework's implementation. [Process](process.md) covers the lifecycle and capabilities of actors. [Node](node.md) explains how actors are managed and how they communicate across networks.
