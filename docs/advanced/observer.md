# Inspecting With Observer

### Installation and starting

To install the `observer` tool, you need to have Golang compiler version 1.20 or higher. Run the following command:

```
$ go install ergo.services/tools/observer@latest
```

Available arguments for starting `observer`:

* **`-help`**: displays information about the available arguments.
* **`-version`**: prints the current version of the Observer tool.
* **`-host`**: specifies the interface name for the Web server to run on (default: `"localhost"`).
* **`-port`**: defines the port number for the Web server (default: `9911`).
* **`-cookie`**: sets the default cookie value used for connecting to other nodes.

<figure><img src="../.gitbook/assets/image (2).png" alt=""><figcaption></figcaption></figure>

If you are running o`bserver` on a server for continuous operation, it is recommended to use the environment variable `COOKIE` instead of the `-cookie` argument. Using sensitive data in command-line arguments is insecure.

After starting `observer`, it initially has no connections to other nodes, so you will be prompted to specify the node you want to connect to.

<figure><img src="../.gitbook/assets/image (4).png" alt=""><figcaption></figcaption></figure>

Once you establish a connection with a remote node, the _Observer application_ main page will open, displaying information about that node.&#x20;

If you have integrated the _Observer application_ into your node, upon opening the _Observer_ page, you will immediately land on the main page showing information about the node where the _Observer application_ was launched.

### Info (main page)

<figure><img src="../.gitbook/assets/image (20).png" alt=""><figcaption></figcaption></figure>

On this tab, you will find general information about the node and the ability to manage its logging level. Changing the logging level only affects the node itself and any newly started processes, but it does not impact processes that are already running.

Graphs provide real-time information over the last 60 seconds, including the total number of processes, the number of processes in the running state, and memory usage data. Memory usage is divided into **used**, which indicates how much memory was reserved from the operating system, and **allocated**, which shows how much of that reserved memory is currently being used by the Golang runtime.

In addition to these details, you can view information about the available loggers on the node and their respective logging levels. For more details, refer to the [Logging](../basics/logging.md) section. Environment variables will also be displayed here, but only if the `ExposeEnvInfo` option was enabled in the `gen.NodeOptions.Security` settings when the inspected node was started.

### Network (main page)

<figure><img src="../.gitbook/assets/image (19).png" alt=""><figcaption></figcaption></figure>

The **Network tab** displays information about the node's network stack.&#x20;

The **Mode** indicates how the network stack was started (_enabled_, _hidden_, or _disabled_).&#x20;

The **Registrar** section shows the properties of the registrar in use, including its capabilities. _Embedded Server_ indicates whether the registrar is running in server mode, while the _Server_ field shows the address and port number of the registrar with which the node is registered.

Additionally, the tab provides information about the default _handshake_ and _protocol versions_ used for outgoing connections.&#x20;

The **Flags** section lists the set of flags that define the functionality available to remote nodes.&#x20;

The **Acceptors** section lists the node's acceptors, with detailed information available for each. This list will be empty if the network stack is running in hidden mode.

Since the node can work with multiple network stacks simultaneously, some acceptors may have different _registrar_ parameters and _handshake_/_protocol_ versions. For an example of simultaneous usage of the Erlang and Ergo Framework network stacks, refer to the [Erlang](../extra-library/network-protocols/erlang.md) section.

<figure><img src="../.gitbook/assets/image (22).png" alt=""><figcaption></figcaption></figure>

The **Connected Nodes** section displays a list of active connections with remote nodes. For each connection, you can view detailed information, including the version of the handshake used when the connection was established and the protocol currently in use. The **Flags** section shows which features are available to the node when interacting with the remote node.

<figure><img src="../.gitbook/assets/image (25).png" alt=""><figcaption></figcaption></figure>

Since the ENP protocol supports a pool of TCP connections within a single network connection, you will find information about the **Pool Size** (the number of TCP connections). The **Pool DSN** field will be empty if this is an incoming connection for the node or if the protocol does not support TCP connection pooling.

Graphs provide a summary of the number of received/sent messages and network traffic over the last 60 seconds, offering a quick overview of communication activity and data flow.

### Process list (main page)

<figure><img src="../.gitbook/assets/image (24).png" alt=""><figcaption></figcaption></figure>

On the **Processes List** tab, you can view general information about the processes running on the node. The number of processes displayed is controlled by the **Start from** and **Limit** parameters.

By default, the list is sorted by the process identifier. However, you can choose different sorting options:

* **Top Running**: displays processes that have spent the most time in the _running_ state.
* **Top Messaging**: sorts processes by the number of sent/received messages in descending order.
* **Top Mailbox**: helps identify processes with the highest number of messages in their mailbox, which can be an indication that the process is struggling to handle the load efficiently.

For each process, you can view brief information:

<figure><img src="../.gitbook/assets/image (26).png" alt=""><figcaption></figcaption></figure>

The **Behavior** field shows the type of object that the process represents.&#x20;

**Application** field indicates the application to which the process belongs. This property is inherited from the parent, so all processes started within an application and their child processes will share the same value.

**Mailbox Messages** displays the total number of messages across all queues in the process's mailbox.

**Running Time** shows the total time the process has spent in the _running_ state, which occurs when the process is actively handling messages from its queue.

By clicking on the process identifier, you will be directed to a page with more detailed information about that specific process.

### Log (main page)

<figure><img src="../.gitbook/assets/image (27).png" alt=""><figcaption></figcaption></figure>

All log messages from the node, processes, network stack, or meta-processes are displayed here. When you connect to the Observer via a browser, the Observer's backend sends a request to the inspector to start a _log process_ with specified logging levels (this _log process_ is visible on the main **Info** tab).

When you change the set of logging levels, the Observer's backend requests the start of a new _log process_ (the old _log process_ will automatically terminate).

To reduce the load on the browser, the number of displayed log messages is limited, but you can adjust this by setting the desired number in the **Last** field.

The **Play/Pause** button allows you to stop or resume the _log process_, which is useful if you want to halt the flow of log messages and focus on examining the already received logs in more detail.

### Process information

<figure><img src="../.gitbook/assets/image (28).png" alt=""><figcaption></figcaption></figure>

This page displays detailed information about the process, including its state, uptime, and other key metrics.

The **fallback parameters** specify which process will receive redirected messages in case the current process's mailbox becomes full. However, if the **Mailbox Size** is unlimited, these fallback parameters are ignored.&#x20;

The **Message Priority** field shows the priority level used for messages sent by this process.

**Keep Network Order** is a parameter applied only to messages sent over the network. It ensures that all messages sent by this process to a remote process are delivered in the same order as they were sent. This parameter is enabled by default, but it can be disabled in certain cases to improve performance.

The **Important Delivery** setting indicates whether the important flag is enabled for messages sent to remote nodes. Enabling this option forces the remote node to send an acknowledgment confirming that the message was successfully delivered to the recipient's mailbox.

The **Compression** parameters allow you to enable message compression for network transmissions and define the compression settings.

Graphs on this page help you assess the load on the process, displaying data over the last 60 seconds.

Additionally, you can find detailed information about any **aliases**, **links**, and **monitors** created by this process, as well as any registered **events** and started **meta-processes**.

The list of environment variables is displayed only if the `ExposeEnvInfo` option was enabled in the node's `gen.NodeOptions.Security` settings.

<figure><img src="../.gitbook/assets/image (34).png" alt=""><figcaption></figcaption></figure>

Additionally, on this page, you can send a message to the process, send an _exit signal_, or even forcibly stop the process using the _kill_ command. These options are available in the context menu.

<figure><img src="../.gitbook/assets/image (1).png" alt=""><figcaption></figcaption></figure>

### Inspect (process page)

<figure><img src="../.gitbook/assets/image (30).png" alt=""><figcaption></figcaption></figure>

If the _behavior_ of this process implements the `HandleInspect` method, the response from the process to the inspect request will be displayed here. The Observer sends these requests once per second while you are on this tab.

In the example screenshot above, you can see the inspection of a process based on `act.Pool`. Upon receiving the inspect request, it returns information about the pool of processes and metrics such as the number of messages processed.

### Log (process page)

<figure><img src="../.gitbook/assets/image (29).png" alt=""><figcaption></figcaption></figure>

The **Log** tab on the process information page displays a list of log messages generated by the specific process.

Please note that since the Observer uses a single stream for logging, any changes to the logging levels will also affect the content of the **Log** tab on the main page.

### Meta-process information

<figure><img src="../.gitbook/assets/image (31).png" alt=""><figcaption></figcaption></figure>

On this page, you'll find detailed information about the meta-process, along with graphs showing data for the last 60 seconds related to incoming/outgoing messages and the number of messages in its mailbox. The meta-process has only two message queues: _main_ and _system_.

You can also send a message to the meta-process or issue an _exit signal_. However, it is not possible to forcibly stop the meta-process using the _kill_ command.

<figure><img src="../.gitbook/assets/image (36).png" alt=""><figcaption></figcaption></figure>

### Inspect (meta-process page)

<figure><img src="../.gitbook/assets/image (32).png" alt=""><figcaption></figcaption></figure>

If the meta-process's _behavior_ implements the `HandleInspect` method, the response from the meta-process to the inspect request will be displayed on this tab. The Observer sends this request once per second while you are on the tab.

### Log (meta-process page)

<figure><img src="../.gitbook/assets/image (33).png" alt=""><figcaption></figcaption></figure>

On the **Log** tab of the meta-process, you will see log messages generated by that specific meta-process. Changing the logging levels will also affect the content of the **Log** tab on the main page.
