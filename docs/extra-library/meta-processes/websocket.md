# WebSocket

This package implements functionality for working with WebSocket connections through two meta-processes: _WebSocket Handler_ (for handling HTTP requests to create WebSocket connections) and _WebSocket Connection_ (for managing the established WebSocket connection). This implementation depends on the external library [https://github.com/gorilla/websocket](https://github.com/gorilla/websocket), which is why it is placed in a separate package `"ergo.services/meta/websocket"`. You can find the source code in the additional meta-process library repository at [https://github.com/ergo-services/meta](https://github.com/ergo-services/meta).

### WebSocket Handler

To handle incoming WebSocket connections, use the `CreateHandler` function. This function returns an object implementing `gen.MetaProcess` and `http.Handler`, needed for launching the meta-process and processing HTTP requests. It accepts `websocket.HandlerOptions` as arguments:

* `ProcessPool`: Specifies a list of process names to handle WebSocket messages.
* `HandshakeTimeout`: Limits the time for the upgrade process.
* `EnableCompression`: Enables compression for WebSocket connections.
* `CheckOrigin`: Allows setting a function to verify the request's origin.

After creation, start the meta-process with `SpawnMeta`. Each connection launches a new _WebSocket Connection_ meta-process for handling.&#x20;

### WebSocket Connection

This package implements the functionality of a meta-process for handling a _WebSocket connection_. It also allows initiating a _WebSocket connection_. To initiate a connection, use the `CreateConnection` function, which takes `websocket.ConnectionOptions` as an argument:

```go
type ConnectionOptions struct {
	Process           gen.Atom
	URL               url.URL
	HandshakeTimeout  time.Duration
	EnableCompression bool
}
```

This structure is similar to `websocket.HandlerOptions` but includes an additional field, **URL**, where the WebSocket server address must be specified. For example:

```go
opt := websocket.ClientOptions{
	URL: url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/ws"},
}
```

During the execution of the `CreateConnection` function, it attempts to establish a _WebSocket connection_ with the specified server.&#x20;

After creating the meta-process, you must launch it using the `SpawnMeta` method from the `gen.Process` interface. If an error occurs while launching the meta-process, you need to call the `Terminate` method from the `gen.MetaBehavior` interface to close the _WebSocket connection_ created by `CreateConnection`:

```go
func(a *MyActor) HandleMessage(from gen.PID, message any) error {
    // ...
    opt := websocket.ConnectionOptions{
	URL: url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/ws"},
    }
    wsconn, err := websocket.CreateConnection(opt)
    if err != nil {
        a.Log().Error("unable to connect to %s: %s", opt.URL.String(), err)
        return nil
    }
    id, err := a.SpawnMeta(wsconn, gen.MetaOptions{})
    if err != nil {
        // unable to spawn meta-process. need to close the connection
        wsconn.Terminate(err)
        a.Log().Error("unable to spawn meta-process: %s", err)
        return nil
    }
    a.Log().Info("spawned meta-process %s to handle connection with %s", id, opt.URL.String())
    // ...
    return nil
}
```

### WebSocket messages

When working with _WebSocket connections_, three types of messages are used:

*   `websocket.MessageConnect` - Sent to the process when the meta-process handling the WebSocket connection is started:\


    ```go
    type MessageConnect struct {
    	ID         gen.Alias
    	RemoteAddr net.Addr
    	LocalAddr  net.Addr
    }
    ```

    \
    The `ID` field in `websocket.MessageConnect` contains the identifier of the meta-process handling the _WebSocket connection_
*   `websocket.MessageDisconnect` - is sent to the process when a _WebSocket connection_ is disconnected:\


    ```go
    type MessageDisconnect struct {
    	ID         gen.Alias
    }
    ```

    \
    The `ID` field in `websocket.MessageDisconnect` contains the identifier of the meta-process that handled the _WebSocket connection_. After the connection is terminated, the meta-process finishes its work and sends this message during its termination phase, signaling that the _WebSocket connection_ has been closed
*   `websocket.Message` - is used for both receiving and sending WebSocket messages:\


    ```go
    type Message struct {
    	ID   gen.Alias
    	Type MessageType
    	Body []byte
    }
    ```

    \
    `ID` - identifier of the meta-process handling the _WebSocket connection_.\
    `Body` - the message's body. \
    `Type` - type of WebSocket message, according to the WebSocket specification. Several message types are available:\


    ```go
    const (
    	MessageTypeText   MessageType = 1
    	MessageTypeBinary MessageType = 2
    	MessageTypeClose  MessageType = 8
    	MessageTypePing   MessageType = 9
    	MessageTypePong   MessageType = 10
    )
    ```

If the meta-process fails to send a message to the process, the _WebSocket connection_ is terminated, and the meta-process itself also terminates.

### Sending a WebSocket-message

To send a message over a _WebSocket connection_, use the `Send` (or `SendAlias`) method from the `gen.Process` interface. The recipient should be the `gen.Alias` of the meta-process handling the _WebSocket connection_.&#x20;

The message must be of type `websocket.Message`. If the `Type` field is not explicitly set, the default is `websocket.MessageTypeText`. The `websocket.Message.ID` field is ignored when sending (it's used only for incoming messages).

For a complete implementation of a WebSocket server, see the project in the repository [https://github.com/ergo-services/examples](https://github.com/ergo-services/examples), the `websocket` project:

<figure><img src="../../.gitbook/assets/image (14).png" alt=""><figcaption></figcaption></figure>



