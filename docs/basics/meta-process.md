# Meta-Process

Meta-processes are designed to integrate synchronous objects into the asynchronous model of Ergo Framework. Although they share some similarities with regular processes, they have a different nature and operational characteristics:

* **Meta-Process Identifier**: meta-process has a process identifier (of type `gen.Alias`) and is associated with its parent process. This allows message routing and synchronous requests to be directed to the meta-process.
* **Sending Messages**: meta-process can send asynchronous messages to other processes or meta-processes (including remote ones) using the `Send` method in the `gen.MetaProcess` interface. However, the sender will always appear as the parent process's `gen.PID`.
* **Handling Messages**: meta-processes can handle asynchronous messages from other processes or meta-processes via the `HandleMessage` callback method of the `gen.MetaBehavior` interface.
* **Handling Synchronous Requests**: can handle synchronous requests from other processes (including remote ones) using the `HandleCall` callback method of the `gen.MetaBehavior` interface.
* **Spawning Other Meta-Processes**: can spawn other meta-processes using the `Spawn` method in the `gen.MetaProcess` interface.
* **Linking and Monitoring**: other processes (including remote ones) can create a link or monitor with the meta-process using the `LinkAlias` and `MonitorAlias` methods of the `gen.Process` interface, referencing the meta-process’s `gen.Alias`.

Despite these similarities with regular processes, meta-processes have distinct characteristics:

* **Concurrency**: meta-process operates with two goroutines. The _main_ goroutine starts when the meta-process is created and handles operations on the synchronous object. The termination of this goroutine leads to the termination of the meta-process. The _auxiliary_ goroutine is launched to handle incoming messages from other processes or meta-processes, and it shuts down when there are no more messages in the meta-process’s mailbox. Therefore, access to the meta-process object’s data is concurrent, as it can be accessed by both goroutines simultaneously. This concurrency must be managed carefully when implementing your own meta-process.
* **Parent Process Dependency**: a meta-process can only be started by a process (or another meta-process) using the `SpawnMeta` method in the `gen.Process` interface (or the `Spawn` method in the `gen.MetaProcess`interface).
* **No Own Environment Variables**: meta-processes do not have their own environment variables. The `Env` and `EnvList` methods of the `gen.MetaProcess` interface return the environment variables of the parent process.
* **Limitations**: meta-processes cannot make synchronous requests, create links, or establish monitors themselves.
* **Termination with Parent**: when the parent process terminates, the meta-process is also automatically terminated.

Meta-processes offer a way to integrate synchronous objects into the asynchronous system while maintaining compatibility with the process communication model. However, due to their concurrent nature and relationship with the parent process, they require careful handling in certain scenarios.

### Ready-to-Use Meta-Processes

Ergo Framework provides several ready-to-use implementations of meta-processes:

* **`meta.TCP`**: allows you to launch a TCP server or create a TCP connection.
* **`meta.UDP`**: for launching a UDP server.
* **`meta.Web`**: for launching an HTTP server and creating an `http.Handler` based on a meta-process.
* **WebSocket Meta-Process**: for working with WebSockets, a WebSocket meta-process is available. Since its implementation has a dependency on `github.com/gorilla/websocket`, it is provided as a separate package: `ergo.services/meta/websocket`.

These meta-processes offer pre-built solutions for integrating common networking and web functionality into your application.

### Meta-process starting

To start a meta-process, the `gen.Process` interface provides the `SpawnMeta` method:

```go
SpawnMeta(behavior gen.MetaBehavior, options genMetaOptions) (gen.Alias, error)
```

If a meta-process needs to spawn other meta-processes, the `gen.MetaProcess` interface implements the `Spawn` method:

```go
Spawn(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error)
```

In the `gen.MetaOptions` options, you can configure:

* **`MailboxSize`**: size of the meta-process’s mailbox for incoming messages.
* **`SendPriority`**: priority for messages sent by the meta-process.
* **`LogLevel`**: the logging level for the meta-process.

Upon successful startup, these methods return the meta-process identifier, `gen.Alias`. This identifier can be used to:

* Send asynchronous messages via the `SendAlias` method in the `gen.Process` interface.
* Make synchronous requests using the `CallAlias` method in the `gen.Process` interface.
* Create links or monitors with the meta-process using the `LinkAlias` or `MonitorAlias` methods in the `gen.Process`interface.

### Meta-process termination

To stop a meta-process, you can send it an _exit_ signal using the `SendExitMeta` method of the `gen.Process` interface. When the meta-process receives this signal, it triggers the callback method `Terminate` from the `gen.MetaBehavior`interface.

Additionally, a meta-process will be terminated in the following cases:

* The parent process has terminated.
* The main goroutine of the meta-process has finished (i.e., the `Start` method of the `gen.MetaBehavior` interface has completed its work).
* The `HandleMessage` (or `HandleCall`) callback method returned an error while processing an asynchronous message (or a synchronous request).
* A panic occurred in any method of the `gen.MetaBehavior` interface.

### Implementing Your Own Meta-Process

To create a custom meta-process in Ergo Framework, you need to implement the `gen.MetaBehavior` interface. This interface defines the behavior of the meta-process and includes the following key methods:

```go
type MetaBehavior interface {
	// callback method for main goroutine
	Start(process MetaProcess) error
	
	// callback methods for the auxiliary goroutine
	HandleMessage(from PID, message any) error
	HandleCall(from PID, ref Ref, request any) (any, error)
	Terminate(reason error)
	HandleInspect(from PID) map[string]string
}
```

* **`Start`**: this method is called when the meta-process starts. It is responsible for initializing the meta-process and running the main logic. The `Start` method runs in the _main_ goroutine of the meta-process, and when it finishes, the meta-process is terminated.
* **`HandleMessage`**: this callback is invoked to handle incoming asynchronous messages from other processes or meta-processes. It runs in the _auxiliary_ goroutine that processes the meta-process’s mailbox.
* **`HandleCall`**: this method processes synchronous requests (calls) from other processes or meta-processes. It is also part of the _auxiliary_ goroutine that processes incoming messages.
* **`Terminate`**: called when the meta-process receives an exit signal or when its parent process terminates. It is responsible for cleanup and finalizing the meta-process before it is removed.

Here is an example of implementing a custom meta-process in Ergo Framework - demonstrates how to create a meta-process that reads data from a network connection and sends it to its parent process:

```go
createMyMeta(conn net.Conn) gen.MetaBehavior {
    return &myMeta{
        conn: conn,
    }
}

type myMeta struct {
    gen.MetaProcess  // Embedding the gen.MetaProcess interface
    conn net.Conn.   // Network connection
}

func (mm *myMeta) Start(meta gen.MetaProcess) error {
    // Assign the meta-process instance to the embedded interface
    mm.MetaProcess = meta
    
    // Ensure that the socket is closed upon exit
    defer close(mm.conn)
    
    // Read data from the socket in a loop
    for {
        buf := make([]byte, 1024)
        n, err mm.conn.Read(buf)
        if err != nil {
            return err // Terminate the meta-process on error
        }
        // Send the data to the parent process
        mm.Send(mm.Parent(), buf[:n])
    }
    return nil // meta-process terminated
}
```

In this example, we embed the `gen.MetaProcess` interface within the `myMeta` struct. This allows us to use the methods of `gen.MetaProcess` (like `Send`, `Parent`, and `Log`) inside the callback methods defined by `gen.MetaBehavior.`

```go
func (mm *myMeta) HandleMessage(from gen.PID, message any) error {
    // Log information about the parent process
    mm.Log().Info("my parent process is %s", mm.Parent())
    // Handle received messages as byte slice
    if data, ok := message.([]byte); ok {
        // Write the data back to the connection
        mm.conn.Write(data)
        return nil
    }
    // Log if the message is of an unknown type
    mm.Log().Info("got unknown message %#v", message)
    return nil
}

func (mm *myMeta) Terminate(reason error) {
    // Close the connection when the meta-process is terminated
    close(mm.conn)
}
```

The `HandleMessage` method processes asynchronous messages sent to the meta-process. It checks the type of the message and takes appropriate actions, such as writing data to the connection. If the method returns an error, it causes the termination of the meta-process, triggering the `Terminate` method to handle the cleanup.

The `Terminate` method ensures that resources like the network connection are properly released when the meta-process ends. If the meta-process encounters an error in `HandleMessage` or `HandleCall`, or if any other termination condition occurs, the `Terminate` method will be called to finalize the shutdown process.

List of available methods in the `gen.MetaProcess` Interface:

```go
type MetaProcess interface {
    ID() Alias           // Returns the ID (gen.Alias) of this meta-process
    Parent() PID         // Returns the PID of the parent process
    Send(to any, message any) error // Sends an asynchronous message to another process or meta-process
    Spawn(behavior MetaBehavior, options MetaOptions) (Alias, error) // Spawns a new meta-process
    Log() Log            // Provides the gen.Log interface for logging
}
```
