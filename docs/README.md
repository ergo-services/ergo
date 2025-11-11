# Overview

Ergo Framework - is an implementation of ideas, technologies, and design patterns from the Erlang world in the Go programming language. It is built on the actor model, network transparency, and a set of ready-to-use components for development. This makes it significantly easier to create complex and distributed solutions while maintaining a high level of reliability and performance.

### Node&#x20;

The management of starting and stopping processes (actors), as well as routing messages between them, is handled by the node. The node is also responsible for network communication. If a message is sent to a remote process, the node automatically establishes a network connection with the remote node where the process is running, thus ensuring network transparency.

### Actors

Actors in Ergo Framework are lightweight processes that live on top of goroutines. Interaction between processes occurs through message passing. Each process has a mailbox with multiple queues to prioritize message handling. Actors can exchange asynchronous messages and also make synchronous requests to each other.

### Networking

Each node has a built-in [Service Discovery](networking/service-discovering.md) function. Typically, the first node launched on a host becomes the registrar for other nodes running on the same host. Upon registration, each node reports its name, the port number for incoming connections, and a set of additional parameters.

[Network transparency](networking/network-transparency.md) in Ergo Framework is achieved through the ENP (Ergo Network Protocol) and EDF (Ergo Data Format). These are based on ideas from Erlang's network stack but are not compatible with it. Therefore, to communicate with Erlang nodes, an additional Erlang package must be used.

To maximize performance in network message exchange, Ergo Framework uses a pool of multiple TCP connections combined into a single logical network connection.

### Performance

Ergo Framework demonstrates high performance in local message exchange due to the use of lock-free queues within the process mailbox and the efficient use of goroutines to handle the process itself. A goroutine is only activated when the process receives a message; otherwise, the process remains idle and does not consume CPU resources.&#x20;

<figure><img src=".gitbook/assets/image (37).png" alt=""><figcaption></figcaption></figure>

You can evaluate the performance yourself using the provided benchmarks [https://github.com/ergo-services/benchmarks](https://github.com/ergo-services/benchmarks).&#x20;

### Requirements

In the development of Ergo Framework, we adhere to the concept of zero dependencies. All functionality is implemented using only the standard Go library. The only requirement for using Ergo Framework is the version of the Go language. Starting from version 3.0, Ergo Framework depends on Go 1.20 or higher.
