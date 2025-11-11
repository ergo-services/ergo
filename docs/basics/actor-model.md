---
description: The Actor Model and Its Properties
---

# Actor Model

The actor model is a computational approach to building concurrent systems, first proposed in the 1970s. At its core is a simple yet powerful idea: instead of having program components share memory and coordinate through locks, they communicate by sending messages to each other.

## The Fundamental Concept

In the actor model, everything is an actor. An actor is an independent entity that has its own private state and processes incoming messages one at a time. Actors never directly access each other's state. Instead, they send messages and wait for responses if needed.

This might seem like a constraint, but it's actually what makes the model powerful. By eliminating shared state, we eliminate entire classes of concurrency bugs that plague traditional multi-threaded programs.

## What Makes an Actor

An actor consists of three things:

**Private State** - Data that belongs exclusively to this actor. No other actor can read or modify it directly.

**Behavior** - The logic that determines how the actor responds to messages. This can change over time as the actor processes different messages.

**Mailbox** - A queue where incoming messages wait to be processed. The actor pulls messages from this queue one at a time.

When an actor receives a message, it can do three things: send messages to other actors, create new actors, or decide how to handle the next message. That's it. Simple, but sufficient to build complex systems.

## Why Sequential Processing Matters

Each actor processes messages sequentially, one after another. This is not a limitation but a design choice that provides important guarantees.

Consider what happens in traditional concurrent programming: multiple threads might access the same data simultaneously. To prevent corruption, you need locks. But locks introduce their own problems - deadlocks, race conditions, and complex reasoning about what state the data is in at any given moment.

Actors sidestep this entirely. Since only one message is processed at a time, the actor's state can only be in one of a finite number of well-defined states. There are no race conditions because there's no race - only one thing happens at a time within an actor.

## Location Transparency

One of the most powerful aspects of the actor model is location transparency. When you send a message to an actor, you don't need to know whether it's running in the same process, on the same machine, or halfway around the world. The semantics are the same.

This makes distribution almost trivial. Code written for a single machine can scale to a distributed system without fundamental changes. The complexity of network communication is handled by the framework, not by your application logic.

## Real-World Implementations

The actor model isn't just theory. It powers real production systems handling massive scale.

**Erlang** pioneered the practical application of the actor model. The language and its BEAM virtual machine have been running telecommunications systems since the 1980s. Systems that need to handle millions of concurrent connections with high reliability naturally gravitate toward Erlang's actor model implementation.

**Akka** brought the actor model to the Java ecosystem. It's used in systems that need to process high-volume transactions, manage complex workflows, or handle real-time data streams. Companies building reactive systems often choose Akka for its proven scalability patterns.

**Orleans** demonstrated that the actor model works well in cloud environments. Its virtual actor pattern, where actors are automatically created and destroyed based on demand, showed how the model adapts to modern distributed computing challenges.

## How This Applies to Go

Go has goroutines and channels, which seem similar to actors and message passing. But there's a crucial difference: goroutines are not isolated. They can share memory, which means you still need locks and face the same concurrency challenges as traditional threading.

Ergo Framework brings true actor model semantics to Go. Each process is an isolated actor. The framework enforces the constraint that actors don't share memory and communicate only through messages. This gives you the benefits of the actor model - no race conditions, simpler concurrent logic, natural distribution - while writing Go code.

The single-goroutine-per-actor constraint might seem limiting at first. In practice, it's liberating. You write sequential code within each actor, and concurrency emerges naturally from having many actors processing messages in parallel.

## The Actor Mindset

Working with the actor model requires a shift in thinking. Instead of thinking about shared data structures protected by locks, you think about independent entities sending messages to each other.

A typical pattern: instead of having multiple threads access a shared cache, you have a cache actor. Want to read from the cache? Send it a message. Want to write? Send a different message. The cache actor processes these requests sequentially, so there's no possibility of corruption. No locks needed.

This pattern scales beautifully. Need more throughput? Add more cache actors, each handling a portion of the key space. Need fault tolerance? Supervise the cache actors, so they restart if they crash. Need distribution? Put cache actors on different machines. The code structure remains the same.

## Moving Forward

The actor model offers a different way to think about concurrent programming. Rather than wrestling with locks and shared memory, you design systems as independent actors exchanging messages. The constraints of the model - sequential processing, message passing only, isolated state - eliminate the complexity that makes traditional concurrent programming difficult.

Ergo Framework brings this programming model to Go. It enforces actor model principles while leveraging Go's strengths: lightweight goroutines, efficient scheduling, and a simple language. The result is a way to build concurrent and distributed systems that's both powerful and approachable.

The following chapters explore how these concepts manifest in Ergo Framework's implementation. [Process](process.md) covers the lifecycle and capabilities of actors. [Node](node.md) explains how actors are managed and how they communicate across networks.
