# UDP

This meta-process allows you to launch a UDP server and handle incoming UDP packets as asynchronous messages of type `meta.MessageUDP`.

To create the meta-process, use the `meta.CreateUDPServer` function. This function takes `meta.UDPServerOptions` as an argument, which specifies the configuration options for the UDP server, such as the host, port, buffer size, and other relevant settings for managing UDP traffic:

```go
type UDPServerOptions struct {
	Host       string
	Port       uint16
	Process    gen.Atom
	BufferSize int
	BufferPool *sync.Pool
}
```

* **Host, Port**: Set the port number and host name for the UDP server.
* **Process**: The name of the process that will handle `meta.MessageUDP` messages. If not specified, all messages will be sent to the parent process. You can also use an `act.Pool` process for distributing message handling.
* **BufferSize**: Sets the buffer size created for incoming UDP messages. For each message, a new buffer of this size is allocated. This field is ignored if `BufferPool` is provided.
* **BufferPool**: Defines a pool for message buffers. The `Get` method should return an allocated buffer of type `[]byte`.

If the meta-process is successfully created, the UDP server will start, and the function `meta.CreateUDPServer` will return `gen.MetaBehavior`.

Next, the created meta-process must be started. It will handle incoming UDP packets and forward them to the process for handling. To start the meta-process, use the `SpawnMeta` method of the `gen.Process` interface.

If an error occurs when starting the meta-process, the UDP server created by the `meta.CreateUDPServer` function must be stopped using the `Terminate` method of the `gen.MetaBehavior` interface.

Stopping the meta-process results in the shutdown of the UDP server and the closing of the socket.

### Sending a UDP-packet

To send a UDP packet, use the `Send` (or `SendAlias`) method from the `gen.Process` interface. The recipient should be the identifier `gen.Alias` of the UDP server meta-process. The message should be of type `meta.MessageUDP`. The `meta.MessageUDP.ID` field is ignored when sending (it is used only for incoming messages).

If a `BufferPool` was used in the UDP server options, the buffer in `meta.MessageUDP.Data` will be returned to the pool after the packet is sent.

For an example UDP server implementation, see the project `demo` at [https://github.com/ergo-services/examples](https://github.com/ergo-services/examples).

<figure><img src="../.gitbook/assets/image (9).png" alt=""><figcaption></figcaption></figure>
