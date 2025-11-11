# TCP

For working with TCP connections, two types of meta-processes are implemented in Ergo Framework:

1. **TCP Server Process**: Responsible for creating a TCP server. It opens the socket and handles incoming connections.
2. **TCP Connection Handler**: Manages established TCP connections, whether they are incoming or client-initiated

### Server

To create a TCP server meta-process, use the `meta.CreateTCPServer` function. It accepts `meta.TCPServerOptions` with the following parameters:

* **Host, Port**: Specify the interface and port for incoming connections.
* **ProcessPool**: Defines a list of worker processes to handle incoming connections. Each new connection is assigned a worker from the pool. If `ProcessPool` is not set, the parent process handles all connections. Using `act.Pool` in `ProcessPool` is not recommended to avoid packet mismanagement.
* **CertManager**: Enables TLS encryption.
* **BufferSize**/**BufferPool**: Manage incoming data buffers.
* **KeepAlivePeriod**/**InsecureSkipVerify**: Handle TCP keep-alive and TLS certificate verification.

After creating the meta-process, you should start it with `SpawnMeta` from the `gen.Process` interface. Example:

```go
type MyTCP struct {
	act.Actor
}

func (t *MyTCP) Init(args ...any) error {
	// create meta-process with TCP server
	opt := meta.TCPServerOptions{
		Port: 17171,
	}
	if metatcp, err := meta.CreateTCPServer(opt); err != nil {
		t.Log().Error("unable to create tcp server meta-process: %s", err)
		return err
	}
	
	// spawn meta-process
	if _, err := t.SpawnMeta(metatcp, gen.MetaOptions{}); err != nil {
		t.Log().Error("unable to spawn tcp server meta-process", err)
		// invoke Terminate to close listening socket
		metatcp.Terminate(err)
		return err
	}
	
	return nil
}
```

If starting the meta-process fails for any reason, you need to free up the port specified in `meta.TCPServerOptions.Port` by using the `Terminate` method of the `gen.MetaBehavior` interface.&#x20;

Upon an incoming connection, the TCP server meta-process creates and starts a new meta-process to handle that connection.

### Connection

To create a client TCP connection, use the `meta.CreateTCPConnection` function. It initiates a TCP connection and, if successful, creates a meta-process to manage it.

The function takes `meta.TCPConnectionOption`, similar to `meta.TCPServerOptions`, but instead of `ProcessPool`, it includes a `Process` field for specifying a process to handle TCP packets. If this field is not used, all data packets are sent to the parent process.

After a successful connection, start the meta-process with `SpawnMeta`. If the process fails, close the connection using the `Terminate` method of the `gen.MetaBehavior` interface..

Example:&#x20;

```go
type Client struct {
	act.Actor
}

func (c *Client) Init(args ...any) error {
	// create meta-process with tcp connection
	opt := meta.TCPConnectionOptions{
		Port: c.port,
	}
	connection, err := meta.CreateTCPConnection(opt)
	if err != nil {
		c.Log().Error("unable to create tcp connection: %s", err)
		return err
	}
	// start meta-process to serve TCP-connection
	id, err := c.SpawnMeta(connection, gen.MetaOptions{})
	if err != nil {
		c.Log().Error("unable to spawn tcp connection meta process: %s", err)
		connection.Terminate(err)
		return err
	}
}
```

### TCP-connection handling

To handle a TCP connection, the meta-process communicates with either the parent or worker process based on the meta-process's launch options. Three types of messages are used:

* `meta.MessageTCPConnect`: Sent when the meta-process starts, containing the connection ID (`ID`), `RemoteAddr`, and `LocalAddr` information.
* `meta.MessageTCPDisconnect`: Sent when the TCP connection is disconnected, including cases where the meta-process is terminated.
* `meta.MessageTCP`: Used for both receiving and sending data.

If the meta-process cannot send a message to the worker process, the connection is terminated, and the meta-process stops working.

### Sending TCP-packet

To send a message to a TCP connection, use the `Send` (or `SendAlias`) method from the `gen.Process` interface. The recipient should be the `gen.Alias` of the meta-process managing the connection. The message must be of type `meta.MessageTCP`. Note that the `meta.MessageTCP.ID` field is ignored during sending (it is only used for incoming messages).

For a detailed example of a TCP server implementation, you can check out the `demo` project in the repository: [https://github.com/ergo-services/examples](https://github.com/ergo-services/examples).

<figure><img src="../.gitbook/assets/image (9).png" alt=""><figcaption></figcaption></figure>

