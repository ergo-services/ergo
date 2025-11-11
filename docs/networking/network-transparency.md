# Network Transparency

The network transparency of Ergo Framework allows processes to exchange messages regardless of whether they are running locally or on remote nodes. The process identifier (`gen.PID`, `gen.ProcessID`, `gen.Alias`) includes the name of the node where the process is running—this information is used by the node to route the message.

If a message is being sent to a process on a remote node, the [_Service Discovery_](service-discovering.md) mechanism helps determine how to establish a network connection to that node. Once the connection is established, the node automatically encodes the message and sends it to the remote node, which then delivers it to the recipient process's mailbox.

The **Ergo Data Format (EDF)** is used for data transmission within Ergo Framework.

### EDF - Ergo Framework Data Format

The _Ergo Data Format (EDF)_ supports encoding and decoding of all scalar types in Golang, as well as specialized types from Ergo Framework (see [Generic Types](../basics/generic-types.md)). To encode custom types, they must be registered using the `edf.RegisterTypeOf` method from the `ergo.services/ergo/net` network stack. The registration of a custom type must occur on both the sender and receiver nodes.

If you're registering a data type that contains another custom type among its child elements, you must register the child type first before registering the parent type.&#x20;

Example:

```go
import ergo.services/ergo/net/edf
...
type MyData struct {
    A int
    B string
    C MyBool
}

type MyBool bool
...

func init() {
    ...
    // type MyBool must be registered first
    edf.RegisterTypeOf(MyBool(false))
    edf.RegisterTypeOf(MyData{})
    ...
}
```

All child elements in the registered structures must be public. If you need to encode/decode structures with private fields, you will need to implement a custom encoder/decoder for that data type. To do this, you must implement the `edf.Marshaler` and `edf.Unmarshaler` interfaces.

These interfaces allow you to define how the custom type is marshaled (encoded) and unmarshaled (decoded), giving you control over the serialization and deserialization process for types with private fields or special requirements.&#x20;

Example:

```go
type myStruct struct{
    a int
    b string    
}

// implementation of edf.Marshaler interface
func (m myStruct) MarshalEDF(w io.Writer) error {
    var encoded []byte
    // do encode
    w.Write(encoded)
    return nil
}

// implementation of edf.Unmarshaler interface
func (m *myStruct) UnmarshalEDF(b []byte) error {
    // do decode
    return nil
}
```

The length of a message encoded using the `MarshalEDF` method must not exceed `2^32` bytes.

In Golang, the `error` type is an interface. To transmit error types over the network, they also need to be registered using the `edf.RegisterError` method. A set of standard errors in Ergo Framework (such as `gen.Err*`, `gen.TerminateReason*`) is automatically registered when the node starts.

Additionally, the following limits are imposed on specific types:

* `gen.Atom`: Maximum length of 255 bytes
* `string`: Maximum length of 65,535 bytes (2^16)
* `binary`: Maximum length of 2^32 bytes
* `map/array/slice`: Maximum number of elements is 2^32

When establishing a connection between nodes, during the handshake phase, the nodes exchange dictionaries containing registered data types and registered errors. These dictionaries are used to reduce the volume of transmitted information during message exchanges, as identifiers are used instead of the names of registered types and errors.

If you register a data type or an error after the connection is established, sending data of that type will result in an error. To make these newly registered types available for network exchange, you will need to disconnect and re-establish the connection.

The _EDF format_ does not support pointers. The use of pointers should remain an internal optimization within your application and should not extend beyond it.

