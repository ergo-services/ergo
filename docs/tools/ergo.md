---
description: Boilerplate Generator for Ergo Framework Projects
---

# Boilerplate Code Generation

The `ergo` tool generates the initial structure and source code for Ergo Framework projects. Instead of writing actor definitions, supervisor specs, and application boilerplate by hand, you describe what you want and the tool writes it for you.

The generated code is plain Go that you own and can modify freely. The tool understands which parts are structural wiring (regenerated as the project grows) and which parts are your business logic (never touched again).

## Installation

Requires Go 1.21 or higher.

```
go install ergo.tools/ergo@latest
```

## Quick Start

Three commands to get a running Ergo node:

```bash
ergo init MyNode github.com/myorg/mynode
cd mynode
go run ./cmd
```

The node starts immediately with an application, a supervisor and an actor, all wired together.

## How It Works

The tool maintains `ergo.yaml` in the project root. This file describes the supervision tree. Every `ergo add` command updates this file and regenerates the affected code.

Each component produces two files:

| File          | Owned by | Regenerated                            | Contains                         |
| ------------- | -------- | -------------------------------------- | -------------------------------- |
| `name_gen.go` | tool     | on every `ergo add` or `ergo generate` | factory, Init spec, Load group   |
| `name.go`     | you      | never                                  | Tune, handlers, Start, Terminate |

User-owned files provide hooks that the generated code calls. The pattern is consistent across all component types:

| File          | Hook                                              | Purpose                              |
| ------------- | ------------------------------------------------- | ------------------------------------ |
| `mysup.go`    | `Tune(spec, args) (SupervisorSpec, error)`        | adjust supervisor spec before start  |
| `myapp.go`    | `Tune(spec, args) (ApplicationSpec, error)` | adjust application spec before start |
| `messages.go` | `extraMessages() []any`                           | register custom EDF message types    |
| `cmd/main.go` | `extraApps() []ApplicationBehavior`               | add external applications            |

## Commands

### ergo init

```
ergo init <NodeName> <module>
```

Creates a new project. The directory name is derived from the last segment of the module path. Generates `ergo.yaml`, all boilerplate, `go.mod`, and runs `go mod tidy`.

```bash
ergo init MyNode github.com/myorg/mynode
ergo init Gateway github.com/acme/api-gateway
```

The default project has one application, one supervisor and one actor, enough to verify everything works before adding real components.

### ergo add actor

```
ergo add actor [--pool] <[Parent:]Name>
```

Adds an actor. `Parent` is the name of an existing supervisor or application. Without a parent the actor is added to `node.processes` and spawned directly by the node at startup.

`--pool` generates a pool actor with a companion worker type. A pool distributes incoming messages across a fixed set of workers and restarts them on failure.

```bash
ergo add actor MySup:MyActor
ergo add actor --pool MySup:RequestPool
ergo add actor StandaloneActor
```

### ergo add supervisor

```
ergo add supervisor <[Parent:]Name> [--type <type>] [--strategy <strategy>]
```

Adds a supervisor. `Parent` is an existing application or supervisor.

The name comes first, and that is not stylistic: `add supervisor`, `add app` and `add message` each read the name from the first argument and look for flags only after it. Lead with a flag and the flag becomes the name - `ergo add supervisor --type one_for_one Sup` records a supervisor literally called `--type`, which then fails during generation. Only `add actor` accepts either order.

`--type` controls which children are restarted when one fails:

| Type                    | Behavior                                                                        |
| ----------------------- | ------------------------------------------------------------------------------- |
| `one_for_one` (default) | only the failed child                                                           |
| `all_for_one`           | all children                                                                    |
| `rest_for_one`          | the failed child and all children started after it                              |
| `simple_one_for_one`    | many instances of one declared spec, spawned at runtime with `StartChild`        |

A supervisor of any type still needs at least one declared child: `Init` rejects an empty `Children` list with "children list can not be empty". A freshly generated supervisor has none, so add a child to it - `ergo add actor MySup:Worker` - before running the node. This bites hardest with `simple_one_for_one`, where it is tempting to assume the instances alone are enough.

`--strategy` controls when a child is restarted:

| Strategy              | Behavior              |
| --------------------- | --------------------- |
| `transient` (default) | only on abnormal exit |
| `permanent`           | always                |
| `temporary`           | never                 |

```bash
ergo add supervisor MyApp:WorkerSup
ergo add supervisor MyApp:CriticalSup --type all_for_one --strategy permanent
ergo add supervisor WorkerSup:SubSup --type rest_for_one
```

### ergo add app

```
ergo add app <Name> [--mode <mode>]
```

Adds an application. `--mode` declares what happens to **the application** when one of its group members terminates. It has nothing to do with stopping the node:

| Mode                  | Behavior                                                                    |
| --------------------- | --------------------------------------------------------------------------- |
| `transient` (default) | the application stops if a member exits abnormally                           |
| `permanent`           | the application stops when any member exits, with that member's reason       |
| `temporary`           | the application stops once the last member is gone, with reason `normal`     |

A stopped application returns to `ApplicationStateLoaded` and the node keeps running. See [Applications](../basics/application.md) for the full lifecycle.

```bash
ergo add app MyApp
ergo add app BackgroundApp --mode temporary
ergo add app CriticalApp --mode permanent
```

### ergo add message

```
ergo add message <Name> --field name:type [--field name:type ...]
```

Adds an EDF message type. Field types can be standard Go types (`string`, `int`, `bool`, `[]byte`) or framework types (`gen.Alias`, `gen.PID`, `gen.Ref`).

Generated struct definitions and EDF registration go into `messages_gen.go`, which is always regenerated when the message list changes.

```bash
ergo add message MessageConnect --field ID:gen.Alias --field Addr:string
ergo add message MessageData --field ID:gen.Alias --field Payload:"[]byte"
```

If a message type has fields of other custom types, add the inner type first. EDF requires nested types to be registered before the types that reference them:

```bash
ergo add message MessageAddress --field City:string --field Street:string
ergo add message MessageUser --field Name:string --field Address:MessageAddress
```

Both nodes must register the same types with identical field definitions. The registration order between nodes does not need to match; nodes negotiate numeric type IDs during handshake.

For detailed coverage of EDF, type constraints, and custom marshaling, see [Network Transparency](../networking/network-transparency.md#edf-ergo-data-format).

### ergo generate

```
ergo generate [ergo.yaml]
```

Regenerates all `*_gen.go` files from `ergo.yaml`. Your `.go` files are never overwritten. Searches for `ergo.yaml` in the current directory and its parents.

```bash
ergo generate
ergo generate /path/to/ergo.yaml
```

## Project Structure

After `ergo init MyNode github.com/myorg/mynode`:

```
mynode/
  ergo.yaml               project definition
  go.mod
  go.sum
  messages_gen.go         EDF struct definitions + registration   (generated)
  messages.go             extraMessages() hook for custom types   (yours)
  apps/
    mynodeapp/
      mynodeapp_gen.go    CreateApp, Load with Group             (generated)
      mynodeapp.go        Tune, Start, Terminate                 (yours)
      mynodesup_gen.go    factory, Init with SupervisorSpec      (generated)
      mynodesup.go        Tune, HandleMessage                    (yours)
      mynodeactor_gen.go  factory                                (generated)
      mynodeactor.go      Init, HandleMessage, HandleCall        (yours)
  cmd/
    main_gen.go           node startup, application list         (generated)
    main.go               extraApps() hook                       (yours)
  README.md
```

The `README.md` is regenerated on every `ergo add` or `ergo generate` and shows the current supervision tree.

## ergo.yaml Reference

```yaml
node:
  name: MyNode
  module: github.com/myorg/mynode
  host: localhost

  network:
    tls: false
    cookie: ""         # empty means auto-generated on every start

  loggers:             # colored, rotate
    - colored

  apps:

    # User-defined application
    - name: MyApp
      mode: transient
      children:
        - sup: MySup
          type: one_for_one
          strategy: transient
          intensity: 2   # max restarts within period
          period: 5      # seconds
          children:
            - actor: MyActor
            - actor: MyPool
              pool: true

    # Known applications from the ergo.services ecosystem
    - observer
    - radar

  processes:           # spawned directly by node, no application
    - actor: StandaloneActor

  messages:
    - name: MessageConnect
      fields:
        - ID: gen.Alias
        - Addr: string
```

Known loggers: `colored` ([docs](../extra-library/loggers/colored.md)), `rotate` ([docs](../extra-library/loggers/rotate.md)).

Known applications: `observer` ([docs](../extra-library/applications/observer.md)), `radar` ([docs](../extra-library/applications/radar.md)).

## Customizing Generated Code

### Supervisor

`mynodesup.go` contains `Tune`, called from the generated `Init`. The generated `Init` builds `SupervisorSpec` from `ergo.yaml` and passes it to `Tune`. Override restart parameters or add dynamic children here:

```go
func (sup *MySup) Tune(spec act.SupervisorSpec, args ...any) (act.SupervisorSpec, error) {
    spec.Restart.Intensity = 10
    spec.Restart.Period = 30
    return spec, nil
}
```

Do not replace `spec.Children` in `Tune` unless you have a specific reason. The children list is populated from `ergo.yaml` by the generated `Init`.

### Application

`mynodeapp.go` contains `Tune`, called from the generated `Load`. The `Group` in `Load` is populated from `ergo.yaml`. Use `Tune` to set metadata, environment variables or dependencies:

```go
func (app *MyApp) Tune(spec gen.ApplicationSpec, args ...any) (gen.ApplicationSpec, error) {
    spec.Description = "main application"
    spec.Version = gen.Version{Release: "1.0.0"}
    spec.Env = map[gen.Env]any{
        "DB_HOST": "localhost",
        "DB_PORT": 5432,
    }
    spec.Depends.Applications = []gen.Atom{"config"}
    return spec, nil
}
```

### Custom EDF Message Types

`messages.go` contains `extraMessages()`, called from the generated `init()`. Add custom types that are not declared in `ergo.yaml`:

```go
func extraMessages() []any {
    return []any{
        MyCustomMessage{},
        AnotherMessage{},
    }
}
```

For types with unexported fields or special encoding needs, implement `edf.Marshaler`/`Unmarshaler` or `encoding.BinaryMarshaler`/`Unmarshaler` in a separate file. See [Network Transparency](../networking/network-transparency.md#edf-ergo-data-format).

### External Applications

Some applications cannot be described in `ergo.yaml` because their constructor requires runtime arguments. Add them in `cmd/main.go`, which is never regenerated:

```go
func extraApps() []gen.ApplicationBehavior {
    return []gen.ApplicationBehavior{
        thirdparty.New(thirdparty.Options{
            DSN:  os.Getenv("DATABASE_URL"),
            Port: 8080,
        }),
    }
}
```

## Typical Workflow

```bash
# 1. Create the project
ergo init OrderService github.com/acme/orders
cd orders

# 2. Verify it runs
go run ./cmd

# 3. Add the supervision tree incrementally
ergo add supervisor MyOrderServiceApp:ApiSup
ergo add actor ApiSup:HttpHandler
ergo add actor --pool ApiSup:RequestPool
ergo add supervisor MyOrderServiceApp:WorkerSup --type all_for_one
ergo add actor WorkerSup:OrderProcessor
ergo add actor WorkerSup:PaymentActor

# 4. Add network message types
ergo add message MessageOrderCreated --field OrderID:string --field Total:int
ergo add message MessageOrderPaid --field OrderID:string

# 5. Implement logic in .go files
# 6. Run, observe, iterate
go run ./cmd
```

Each `ergo add` updates `ergo.yaml`, regenerates `*_gen.go` files, and leaves your `.go` files untouched.

## What's Next

* [Observer](../extra-library/applications/observer.md): web UI, API and MCP surface for inspecting running nodes and processes
* [Actors](https://github.com/ergo-services/ergo/blob/v330/docs/actors/README.md): actor types, supervision and messaging patterns
* [Applications](../basics/application.md): application lifecycle and modes
* [Pool](../actors/pool.md): distributing work across worker processes
* [Network Transparency](../networking/network-transparency.md): EDF serialization and distributed messaging
