---
description: The Actor Model and Its Properties
---

# Actor Model

The actor model is a computational approach to building concurrent systems, first proposed in the 1970s. Instead of sharing memory and coordinating through locks, components communicate by sending messages to each other.

## The Fundamental Concept

In the actor model, everything is an actor. An actor is an independent entity that has its own private state and processes incoming messages one at a time. Actors never directly access each other's state. Instead, they send messages and wait for responses if needed.

Without shared state, the concurrency bugs that come from unsynchronized access to it cannot occur.

## What Makes an Actor

An actor consists of three things:

**Private State** - Data that belongs exclusively to this actor. No other actor can read or modify it directly.

**Behavior** - The logic that determines how the actor responds to messages. This can change over time as the actor processes different messages.

**Mailbox** - A queue where incoming messages wait to be processed. The actor pulls messages from this queue one at a time.

When an actor receives a message, it can do three things: send messages to other actors, create new actors, or decide how to handle the next message. That's it.

## Why Sequential Processing Matters

Each actor processes messages sequentially, one after another.

Consider what happens in traditional concurrent programming: multiple threads might access the same data simultaneously. To prevent corruption, you need locks. But locks introduce their own problems - deadlocks, race conditions, and complex reasoning about what state the data is in at any given moment.

Since only one message is processed at a time, the actor's state can only be in one of a finite number of well-defined states. Within an actor there is nothing to race: only one thing happens at a time.

## Location Transparency

When you send a message to an actor, you don't need to know whether it's running in the same process, on the same machine, or halfway around the world. The addressing and the API are the same.

The same messaging API is used for local and remote processes, so code written for a single machine runs unchanged across several. Remote delivery keeps its own failure semantics: the connection can drop, and the peer can be gone before the message arrives.

## Real-World Implementations

**Erlang** pioneered the practical application of the actor model. The language and its BEAM virtual machine have been running telecommunications systems since the 1980s.

**Akka** brought the actor model to the Java ecosystem. It's used in systems that need to process high-volume transactions, manage complex workflows, or handle real-time data streams.

**Orleans** applies the actor model in cloud environments. Its virtual actor pattern creates and destroys actors on demand.

## How This Applies to Go

Go has goroutines and channels, which seem similar to actors and message passing. But there's a crucial difference: goroutines are not isolated. They can share memory, which means you still need locks and face the same concurrency challenges as traditional threading.

Ergo Framework brings actor model semantics to Go. Each process is an actor with its own goroutine and its own mailbox, and the only way in is a message. Logic inside an actor stays sequential, while actors run concurrently with each other.

One boundary the language cannot enforce for us, and it is worth knowing from the start: **memory isolation is a discipline the framework supports, not a constraint it imposes.** A message between two processes on the same node is handed over as the Go value it is, with no copy and no serialization. Send a map, a slice or a pointer and both processes then hold the same memory, and Go's race detector will say so. Only a message that crosses a node boundary is encoded, and the encoding is what makes the copy.

So "no shared state" is yours to keep: send values rather than references, or treat a send as handing over ownership and stop touching what you sent. The framework ships a vet tool, [argus](../tools/argus.md), whose A1001 rule flags exactly this.

## The Actor Mindset

Working with the actor model requires a shift in thinking. Instead of thinking about shared data structures protected by locks, you think about independent entities sending messages to each other.

A typical pattern: instead of having multiple threads access a shared cache, you have a cache actor. Want to read from the cache? Send it a message. Want to write? Send a different message. The cache actor processes these requests sequentially, so there's no possibility of corruption. No locks needed.

Need more throughput? Add more cache actors, each handling a portion of the key space. Need fault tolerance? Supervise the cache actors, so they restart if they crash. Need distribution? Put cache actors on different machines. The code structure remains the same.

## Moving Forward

The following chapters explore how these concepts manifest in Ergo Framework's implementation. [Process](process.md) covers the lifecycle and capabilities of actors. [Node](node.md) explains how actors are managed and how they communicate across networks.
