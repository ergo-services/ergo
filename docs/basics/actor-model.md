---
description: The Actor Model and Its Properties
---

# Actor Model

### Intro

The actor model was developed in the 1970s for concurrent and parallel computations. It defines some key rules for how system components should behave and interact with each other. The main idea is that interactions between program components are not conducted through function or procedure calls but through the exchange of asynchronous messages. You can read more about the actor model and its history in the [Wikipedia article](https://en.wikipedia.org/wiki/Actor\_model).

One of the most popular programming languages that uses this model is **Erlang**, where the actor model is at the core of its BEAM virtual machine. In Erlang, actors are represented by lightweight processes within the virtual machine.

There are also implementations of this model in other languages. For example, in **Java**, the **Akka** framework provides a powerful tool for building solutions based on the actor model. Akka brings the principles of the actor model to the Java ecosystem, enabling developers to create highly concurrent, distributed, and fault-tolerant applications.

### Actor in Ergo Framework

The Ergo Framework implements the actor model based on lightweight processes. Each process can send and receive asynchronous messages, make synchronous calls, and spawn new processes.&#x20;

A process in Ergo Framework is a lightweight entity running on top of a goroutine, built around the actor model. Each process has a mailbox for incoming messages. By default, this mailbox is of unlimited size, but it can be limited by setting an appropriate parameter when the process is started. The mailbox contains four queues: _Main_, _System_, _Urgent_, and _Log_. These queues determine the priority of message processing.

An actor in Ergo Framework is an abstraction over such a process, with a set of callbacks for handling incoming messages. The standard library includes general-purpose actors like `act.Actor`, `act.Supervisor`, and `act.Pool`, as well as a specialized actor for handling HTTP requests, `act.WebHandler`.&#x20;

Mailbox processing in an actor is done sequentially in a FIFO order, but with respect to priority. Messages from the _Urgent_ queue are processed first, followed by those in the _System_ queue. If these queues are empty, messages from the _Main_ queue are processed. Typically, messages from the node (e.g., a request to stop the process) are placed in the _Urgent_ and _System_ queues. All other messages are delivered to the _Main_ queue by default. Messages in the _Log_ queue are processed with the lowest priority.



