---
description: Erlang network stack
---

# Erlang

This package implements the Erlang network stack, including the DIST protocol, ETF data format, EPMD registrar functionality, and the Handshake mechanism.

It is compatible with OTP-23 to OTP-29. The source code is available on the project's GitHub page at [https://github.com/ergo-services/proto](https://github.com/ergo-services/proto) in the `erlang23` directory.

The source code is distributed under the MIT License, like the rest of the framework, and is free to use in commercial projects without restrictions.

### EPMD

The `epmd` package implements the `gen.Registrar` interface. To create it, use the `epmd.Create` function with the following options:

* **Port**: Registrar port number (default: `4369`).
* **EnableRouteTLS**: Enables TLS for all `gen.Route` responses on resolve requests. This is necessary if the Erlang cluster uses TLS.
* **DisableServer**: Disables the internal server mode, useful when using the Erlang-provided `epmd` service.

To use this package, include `ergo.services/proto/erlang23/epmd`.

### Handshake

The `handshake` package implements the `gen.NetworkHandshake` interface. To create a handshake instance, use the `handshake.Create` function with the following options:

* **Flags**: Defines the supported functionality of the Erlang network stack. The default is set by `handshake.DefaultFlags()`.
* **UseVersion5**: Enables handshake version 5 mode (default is version 6).

To use this package, include `ergo.services/proto/erlang23/handshake`.

### DIST protocol

The `ergo.services/proto/erlang23/dist` package implements the `gen.NetworkProto` and `gen.Connection` interfaces. To create it, use the `dist.Create` function and provide `dist.Options` as an argument, where you can specify the `FragmentationUnit` size in bytes. This value is used for fragmenting large messages. `65000` bytes is both the default and the **floor**: a smaller value is silently raised to it, with no error and no log line, so asking for 8000 gets you 65000.

The Erlang DIST proto deliberately does **not** implement `gen.TypeRegistry`, because the Erlang external term format (ETF) carries primitives, atoms, lists, tuples, and binaries directly on the wire without a separate type-registration step. Use `etf.RegisterTypeOf` (described below) to teach the Erlang decoder how to map incoming tuples or atoms to your Go types.

Note what that means for `node.Network().RegisterType` and for `ApplicationSpec.Network.RegisterTypes`: they do not reach the Erlang wire, and they do not tell you so. A node always keeps the native proto registered alongside whatever you configured, so those calls find a TypeRegistry to write into and return `nil` even on a node that speaks nothing but DIST. An application whose spec lists its wire types therefore loads without complaint, and the types are in the EDF registry that this node never uses. For the Erlang side, `etf.RegisterTypeOf` is the only registration that counts.

To use this package, include `ergo.services/proto/erlang23/dist`.

### ETF data format

Erlang uses the _ETF (Erlang Term Format)_ for encoding messages transmitted over the network. Due to differences in data types between Golang and Erlang, decoding received messages involves converting the data to their corresponding Golang types:

* `number` -> `int64`
* `float` number -> `float64`
* `big number` -> `big.Int`  from `math/big`, or to `int64`/`uint64`
* `map` -> `etf.Map` (`map[any]any`)
* `binary` -> `[]byte`
* `list` -> `etf.List` (`[]any`), **or `string`** - see below
* `tuple` -> `etf.Tuple` (`[]any`) or a registered struct type
* `atom` -> `gen.Atom`
* `pid` -> `gen.PID`
* `ref` -> `gen.Ref`
* `ref` (alias) -> `gen.Alias`
* `atom` = true/false -> `bool`

These are named types, and a type assertion is exact: a map arrives as `etf.Map`, so `.(map[any]any)` fails on it even though `etf.Map` is defined as `map[any]any`. The same goes for numbers - every Erlang integer is an `int64`, so `.(int)` never matches. Neither mistake produces an error you can see; the assertion just reports `false` and your code takes whatever branch it has for bad input.

**Erlang has no string type, and that leaks into Go.** A list of integers between 0 and 255 is sent as `STRING_EXT` and arrives as a Go `string`; any other list arrives as an `etf.List`. So `"hello"` from an Erlang shell is a Go `string`, and so is `[1,2,3]` - it reaches you as `"\x01\x02\x03"`. Meanwhile `[1000,2000]` is an `etf.List{1000, 2000}`, because those values do not fit a byte. Nothing announces which of the two you are getting.

`etf.TermToString` exists for exactly this: it accepts `string`, `etf.List`, `[]byte` and `gen.Atom` and returns the text, so a callback that wants a string can stop caring which shape arrived. When you control both sides, sending a binary (`<<"hello">>`) instead of a charlist removes the ambiguity altogether: it always arrives as `[]byte`.

When encoding data in the _Erlang ETF format_:

* `map` -> `map` `#{}`
* `slice`/`array` -> `list` `[]`
* `struct` -> `map` with field names as keys (considering `etf:` tags on struct fields)
* registered type of `struct` -> `tuple` with the first element being the registered struct name, followed by field values in order.
* `[]byte` -> `binary`
* `int*`/`float*`/`big.Int` -> `number`
* `string` -> a charlist (`STRING_EXT`), which is what Erlang calls a string. Above 65535 bytes the encoder refuses it with `etf.ErrStringTooLong`
* `etf.String` -> `binary`, for when you want `<<"...">>` on the Erlang side
* `etf.Charlist` -> a charlist encoded from `[]rune`, so text outside Latin-1 survives
* `gen.Atom` -> `atom`
* `gen.PID` -> `pid`
* `gen.Ref` -> `ref`
* `gen.Alias -> ref` (alias)
* `bool` -> `atom` true/false

You can also use the functions `etf.TermIntoStruct` and `etf.TermProplistIntoStruct` for decoding data. These functions take into account `etf:` tags on struct fields, allowing the values to map correctly to the corresponding struct fields when decoding `proplist` data.

To automatically decode data into a struct, you can register the struct type using `etf.RegisterTypeOf`. This function takes the object of the type being registered and decoding options `etf.RegisterTypeOptions`. The options include:

* `Name` - The name of the registered type. By default it is taken from the `reflect` package as `#` followed by the package path and the type name, for example `#github.com/myorg/myapp/MyValue`
* `Strict` - Determines whether the data must match the struct. With `Strict: false` non-matching data is decoded into `any`. With `Strict: true` a mismatch **panics** during decoding - an overflow or a wrong destination type raises it - so do not enable it for input you do not control.

To be automatically decoded the data sent from Erlang must be a tuple, with the first element being an atom whose value matches the type name registered in Golang. For example:

```go
type MyValue struct{
    MyString string
    MyInt    int32
}

...
// register type MyValue with name "myvalue"
etf.RegisterTypeOf(MyValue{}, etf.RegisterTypeOptions{Name: "myvalue", Strict: true})
...
```

The values sent by an Erlang process should be in the following format:

```erlang
> erlang:send(Pid, {myvalue, "hello", 123}).
```

### Ergo-node in Erlang-cluster

If you want to use the Erlang network stack by default in your node, you need to specify this in `gen.NetworkOptions` when starting the node:

```go
import (
    "fmt"
    
    "ergo.services/ergo"
    "ergo.services/ergo/gen"
    "ergo.services/proto/erlang23/dist"
    "ergo.services/proto/erlang23/epmd"
    "ergo.services/proto/erlang23/handshake"
)

func main() {
    var options gen.NodeOptions
    
    // set cookie
    options.Network.Cookie = "123"
    
    // set Erlang Network Stack for this node
    options.Network.Registrar = epmd.Create(epmd.Options{})
    options.Network.Handshake = handshake.Create(handshake.Options{})
    options.Network.Proto = dist.Create(dist.Options{})

    // starting node
    node, err := ergo.StartNode(gen.Atom(OptionNodeName), options)
    if err != nil {
        fmt.Printf("Unable to start node '%s': %s\n", OptionNodeName, err)
        return
    }
    
    node.Wait()
}
```

In this case, all outgoing and incoming connections will be handled by the Erlang network stack. For a complete example, see the [Erlang step of the tour](https://github.com/ergo-services/examples/tree/master/tour/09-erlang): an application that Erlang drives through `gen_server:call`, with the type mapping above put to work in one place.

<figure><img src="../../.gitbook/assets/image.png" alt=""><figcaption></figcaption></figure>

#### `net_adm:ping` says `pang`

The first thing anyone tries from the Erlang shell is a ping, and it fails on a node that works perfectly:

```erlang
1> net_adm:ping('ergo@localhost').
pang
```

A `gen_server:call` to a process on that same node, typed immediately afterwards, answers normally.

`ping` does not test the connection. It calls the `net_kernel` process on the other node with `{is_auth, node()}` and expects `yes`; on anything else it runs `erlang:disconnect_node` and returns `pang`. An Ergo node has no `net_kernel`, so nothing answers - and the disconnect is why a connection may appear in the log and go again just before the one you actually use. Judge the link by whether messages arrive, not by `ping`.

The rest of Erlang's introspection is the same story in reverse: `observer:start()`, `recon` and the shell's process listings read structures that only a BEAM node has. An Ergo node is not a BEAM node, and the [Observer](../applications/observer.md) application does not read an Erlang one either - it inspects nodes through the `system` application every Ergo node runs, which an Erlang node does not have. Each side keeps its own tools; what crosses between them is messages.

If you want to maintain the ability to accept connections from Ergo nodes while using the Erlang network stack as a main one, you need to add an acceptor in the `gen.NetworkOptions` settings:

```go
import (
    "fmt"
    "ergo.services/ergo"
    "ergo.services/ergo/gen"
    
    // Ergo Network Stack
    hs "ergo.services/ergo/net/handshake"
    "ergo.services/ergo/net/proto"
    "ergo.services/ergo/net/registrar"

    // Erlang Network Stack    
    "ergo.services/proto/erlang23/dist"
    "ergo.services/proto/erlang23/epmd"
    "ergo.services/proto/erlang23/handshake"
)

func main() {
    ...
    acceptorErlang := gen.AcceptorOptions{}
    acceptorErgo := gen.AcceptorOptions{
        Registrar: registrar.Create(registrar.Options{}),
        Handshake: hs.Create(hs.Options{}),
        Proto:     proto.Create(),
    }
    options.Network.Acceptors = append(options.Network.Acceptors, 
                                    acceptorErlang, acceptorErgo)
    // starting node
    node, err := ergo.StartNode(gen.Atom(OptionNodeName), options)
```

Please note that if the list of acceptors is empty when starting the node, it will launch an acceptor with the network stack using `Registrar`, `Handshake`, and `Proto` from `gen.NetworkOptions`.

If you set `options.Network.Acceptors`, you must explicitly define the parameters for all necessary acceptors. In the example, `acceptorErlang` is created with empty `gen.AcceptorOptions` (the Erlang stack from `gen.NetworkOptions` will be used), while for `acceptorErgo`, the Ergo Framework stack (`Registrar`, `Handshake`, and `Proto`) is explicitly defined.

In this example, you can establish incoming and outgoing connections using the Erlang network stack. However, the Ergo Framework network stack can only be used for incoming connections. To create outgoing network connections using the Ergo Framework stack, you need to configure a static route for a group of nodes by defining a match pattern:

```go
...
// starting node
node, err := ergo.StartNode(gen.Atom(OptionNodeName), options)
// add static route  
route := gen.NetworkRoute{
    Resolver: acceptorErgo.Registrar.Resolver(),
}
match := ".ergonodes.local"
if err := node.Network().AddRoute(match, route, 1); err != nil {
    panic(err)
}
```

For more detailed information, please refer to the [Static Routes](../../networking/static-routes.md) section.

### Erlang-node in Ergo-cluster

If your cluster primarily uses the Ergo Framework network stack by default and you want to enable interaction with Erlang nodes, you'll need to add an acceptor using the Erlang network stack. Additionally, you must define a static route for Erlang nodes using a match pattern:

```go
import (
    "fmt"
    
    "ergo.services/ergo"
    "ergo.services/ergo/gen"
    "ergo.services/proto/erlang23/dist"
    "ergo.services/proto/erlang23/epmd"
    "ergo.services/proto/erlang23/handshake"
)

func main() {
    var options gen.NodeOptions
    
    // set cookie
    options.Network.Cookie = "123"
    
    // add acceptors
    acceptorErgo := gen.AcceptorOptions{}
    acceptorErlang := gen.AcceptorOptions{
        Registrar: epmd.Create(epmd.Options{}),
        Handshake: handshake.Create(handshake.Options{}),
        Proto:     dist.Create(dist.Options{}),
    }
    options.Network.Acceptors = append(options.Network.Acceptors, 
                                    acceptorErgo, acceptorErlang)

    // starting node
    node, err := ergo.StartNode(gen.Atom(OptionNodeName), options)
    if err != nil {
        fmt.Printf("Unable to start node '%s': %s\n", OptionNodeName, err)
        return
    }
    
    // add static route  
    route := gen.NetworkRoute{
        Resolver: acceptorErlang.Registrar.Resolver(),
    }
    if err := node.Network().AddRoute(".erlangnodes.local", route, 1); err != nil {
        panic(err)
    }
    
    node.Wait()
}
```

### Actor `GenServer`

The `erlang23.GenServer` actor implements the low-level `gen.ProcessBehavior` interface, enabling it to handle messages and synchronous requests from processes running on an Erlang node. The following message types are used for communication in Erlang:

* regular messages - sent from Erlang using `erlang:send` or the `Pid ! message` syntax
* cast-messages - sent from Erlang with `gen_server:cast`
* call-requests - from Erlang made with `gen_server:call`

`erlang23.GenServer` uses the `erlang23.GenServerBehavior` interface to interact with your object. This interface defines a set of callback methods for your object, which allow it to handle incoming messages and requests. All methods in this interface are optional, meaning you can choose to implement only the ones relevant to your specific use case:

```go
type GenServerBehavior interface {
	gen.ProcessBehavior

	Init(args ...any) error
	HandleInfo(message any) error
	HandleCast(message any) error
	HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)
	Terminate(reason error)

	HandleEvent(message gen.MessageEvent) error
	HandleInspect(from gen.PID, item ...string) map[string]string
}
```

The callback method `HandleInfo` is invoked when an asynchronous message is received from an Erlang process using `erlang:send` or via the `Send*` methods of the `gen.Process` interface. The `HandleCast` callback method is called when a cast message is sent using `gen_server:cast` from an Erlang process. Synchronous requests sent with `gen_server:call` or `Call*` methods are handled by the `HandleCall` callback method.

If your actor only needs to handle regular messages from Erlang processes, you can use the standard `act.Actor` and process asynchronous messages in the `HandleMessage` callback method.

To start a process based on `erlang23.GenServer`, create an object embedding `erlang23.GenServer` and implement a factory function for it.

Example:

```go
import "ergo.services/proto/erlang23"

func factory_MyActor() gen.ProcessBehavior {
    return &MyActor{}
}

type MyActor struct {
    erlang23.GenServer
}
```

To send a cast message, use the `Cast` method of `erlang23.GenServer`.

```go
func (ma *MyActor) HandleInfo(message any) error {
    ...
    ma.Cast(Pid, "cast message")
    return nil
}
```

To send regular messages, use the `Send*` methods of the embedded `gen.Process` interface. Synchronous requests are made using the `Call*` methods of the `gen.Process` interface.

Like `act.Actor`, an actor based on `erlang23.GenServer` supports the `TrapExit` functionality to intercept exit signals. Use the `SetTrapExit` and `TrapExit` methods of your object to manage this functionality, allowing your process to handle exit signals rather than terminating immediately when receiving them.
