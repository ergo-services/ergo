---
description: Data Types and Interfaces Used in Ergo Framework
---

# Generic Types

## Data types

### gen.Atom

This type is an alias for the `string` type. It was introduced as a specialized string that allows differentiation between regular strings and those used for node names or process names. In the network stack, this type is also handled separately and is actively used in the Atom-cache and Atom-mapping mechanisms. When printed, values of this type are enclosed in single quotes.

```go
fmt.Printf("%s", gen.Atom("hello"))
...
'hello'
```

### gen.PID

This type is used as a process identifier. It is a structure containing several fields: the node name `Node`, a unique sequential number within the node `ID`, and a `Creation` field.

When printed as a string, this type is transformed into the following representation:

```go
pid := gen.PID{Node:"t1node@localhost", ID:1001, Creation:1685523227}
fmt.Printf("%s", pid)
...
<90A29F11.0.1001>
```

The node name is encoded into a hash using the CRC32 algorithm.

### gen.Ref

The node can generate unique identifiers. For this purpose, the `gen.Node` interface provides the `MakeRef` method. It returns a guaranteed unique value of type `gen.Ref`. These values are used as unique request identifiers in synchronous calls and as unique tokens when registering a `gen.Event` by a producer process.

When printed as a string, the value is represented as:

```go
ref := gen.Ref{
    Node:"t1node@localhost", 
    Creation:1685524098, 
    ID:[3]uint32{0x1f4c2, 0x5d90, 0x0},
}
fmt.Printf("%s", ref)
...
Ref#<90A29F11.128194.23952.0>
```

### gen.ProcessID

This type is used as a process identifier with an associated name. It is a structure containing two fields: the process name `Name` and the node name `Node`.

When printed as a string, the node name is transformed into a hash using the CRC32 algorithm, resulting in the following representation:

```go
process := gen.ProcessID{Name:"example", Node:"t1node@localhost"}
fmt.Printf("%s", process)
...
<90A29F11.'example'>
```

### gen.Alias

This type is an alias for the `gen.Ref` type. Values of type `gen.Alias` are used as temporary process identifiers, created using the `CreateAlias` method in the `gen.Process` interface, as well as identifiers for [meta-processes](meta-process.md).

When printed as a string, the representation is similar to `gen.Ref`, but with a different prefix:&#x20;

```go
alias := gen.Alias{
    Node:"t1node@localhost", 
    Creation:1685524098, 
    ID:[3]uint32{0x1f4c2, 0x5d90, 0x0},
}
fmt.Printf("%s", ref)
...
Alias#<90A29F11.128194.23952.0>
```

### gen.Event

Values of this type are used when subscribing to events through the `MonitorEvent` and `LinkEvent` methods. This type is a structure similar to `gen.ProcessID`, containing two fields: `Name` and `Node`.

When printed as a string, the representation is similar to `gen.ProcessID`, but with an added prefix:

```go
event := gen.Event{Name:"event1", Node:"t1node@localhost"}
fmt.Printf("%s", event)
...
Event#<90A29F11:'event1'>
```

### gen.Env

This type is an alias for the `string` type. It is used for the names of environment variables for nodes and processes. In Ergo Framework, environment variable names are case-insensitive.

When printed as a string, the value of this type is converted to uppercase:

```go
env := gen.Env("name1")
fmt.Printf("%s", env)
...
NAME1
```

## Interfaces

### gen.Node

To start a node, use the function `ergo.StartNode(...)`. If the node starts successfully, it returns the `gen.Node` interface. This interface provides a set of functions for interacting with the node. The full list of methods can be found in the reference documentation.

### gen.Process

This type is an interface to a process object. In Ergo Framework, this interface is embedded in the actor `act.Actor`, so all methods of this interface become available to objects based on `act.Actor` (and its derivatives).

```go
type myActor struct {
    act.Actor
}

func (a *myActor) Init(args ...any) error {
    // Sending a message to itself using methods PID and Send 
    // that belong to the embedded gen.Process interface
    a.Send(a.PID(), "hello")
}
```

The full list of available methods for the `gen.Process` interface can be found in the reference documentation.

### gen.Network

You can access this interface using the `Network` method of the `gen.Node` interface. This interface provides a set of methods for managing the node's network stack. The full list of available methods for this interface can be found in the reference documentation.

### gen.RemoteNode

This interface allows you to retrieve information about a remote node with which a connection has been established, as well as to spawn processes (using the `Spawn` and `SpawnRegister` methods) and start applications on it (using the `ApplicationStart*` methods). You can access this interface through the `GetNode` and `Node` methods of the `gen.Network`interface.
