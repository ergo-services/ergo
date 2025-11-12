---
description: Logging system and logger implementations
---

# Logging

Understanding what happens inside a running system requires logging. But logging in distributed actor systems isn't straightforward. Messages pass between dozens of processes. Processes spawn dynamically, handle requests, and terminate. Network connections form and break. Following a single request's path through the system means tracking its journey across multiple processes, possibly across multiple nodes.

Traditional logging compounds the problem. Each component writes to its own log. Process logs go to one file, network logs to another, node events to a third. When something goes wrong, you're piecing together a timeline from scattered sources, correlating by timestamp and hoping you've found all the relevant entries. It's detective work when you need diagnostic clarity.

Ergo Framework centralizes the logging flow while keeping distribution flexible. Every log call - whether from a process, meta process, or the node itself - flows through a single logging system. That system distributes messages to registered loggers based on configurable filters. One logger might write everything to the console. Another might write only errors to a file. A third might send metrics to a monitoring system. The architecture is simple: centralized input, filtered distribution to multiple outputs.

## How Messages Flow

When code calls `process.Log().Info("message")`, the framework creates a `gen.MessageLog` structure. This contains the timestamp, severity level, source identifier, message format and arguments, and any attached structured fields. The message enters the node's logging subsystem.

The subsystem maintains loggers organized by severity level. Each logger, when registered, declares which levels it handles - perhaps just errors and panics, perhaps everything from debug upward. When a log message arrives, the subsystem looks up which loggers are registered for that message's level and calls their `Log` methods.

This is fan-out distribution. A single info-level message goes to every logger registered for info level. The default logger writes it to stdout. A file logger appends it to a file. A metrics logger counts it. Each logger receives the same message and processes it independently.

**Hidden Loggers** (introduced in v3.2.0) - Prefix a logger name with "`.`" to create a hidden logger that's excluded from fan-out. Hidden loggers only receive logs from processes that explicitly call `SetLogger(name)`. This creates truly isolated logging streams - bidirectional isolation. For example, register `".debug"` as a hidden logger, then have a specific process use `SetLogger(".debug")`. That process's logs go only to the hidden logger (not to other loggers), and the hidden logger receives logs only from that process (not from fan-out). This is useful for separating verbose debugging output or creating per-process log files without mixing logs from other processes.

You can also use `SetLogger("filename")` to send a process's logs to a specific logger. The process's logs go only to that logger, but the logger still receives fan-out logs from other processes. This routes verbose process logs to a dedicated destination but doesn't create isolation - the logger sees both the process's logs and system-wide fan-out.

## Severity Levels

The framework provides six severity levels, ordered from most to least verbose:

`gen.LogLevelTrace` - Framework internals, message routing, network packets. Extremely verbose, intended only for deep debugging of the framework itself.

`gen.LogLevelDebug` - Application debugging information. Useful during development but typically disabled in production.

`gen.LogLevelInfo` - Normal informational messages. This is the default level. Startup events, request handling, normal operations.

`gen.LogLevelWarning` - Conditions that merit attention but don't prevent operation. Deprecated API usage, approaching resource limits, retry scenarios.

`gen.LogLevelError` - Errors that prevent specific operations but don't crash the system. Failed requests, unavailable resources, validation failures.

`gen.LogLevelPanic` - Critical errors requiring immediate attention. Despite the name, logging at this level doesn't trigger a panic - it's just the highest severity marker.

Setting a level creates a threshold. Set a process to `gen.LogLevelWarning` and it logs warnings, errors, and panics, but suppresses info, debug, and trace. Each level implicitly includes all higher severity levels.

Two special levels control behavior rather than representing severity:

`gen.LogLevelDefault` - Sentinel meaning "inherit." Nodes with this level become `gen.LogLevelInfo`. Processes with this level inherit from their parent, leader, or node. This default-then-inherit pattern allows hierarchical log level configuration.

`gen.LogLevelDisabled` - Stops all logging from the source. The framework doesn't even create log messages. Use this to completely silence a source without removing loggers.

Trace deserves special mention. It's so verbose that enabling it accidentally could flood storage. You can't enable it dynamically via `SetLevel`. It must be set at startup through `gen.NodeOptions.Log.Level` or `gen.ProcessOptions.LogLevel`. This restriction prevents operational mistakes.

The node starts at `gen.LogLevelInfo`. Processes inherit this unless their spawn options specify otherwise. After startup, you can adjust a process's level dynamically with `SetLevel`, allowing surgical verbosity changes during debugging.

## Identifying Log Sources

The logging subsystem differentiates between four source types: node, process, meta process, and network. Each carries its source information in a typed structure - `gen.MessageLogNode`, `gen.MessageLogProcess`, `gen.MessageLogMeta`, or `gen.MessageLogNetwork`. This typing allows custom loggers to handle different sources differently, perhaps routing network logs to one destination and process logs to another.

The default logger formats each source type distinctly in its output:

**Node logs** show the node name as a CRC32 hash:
```
2024-07-31 07:53:57 [info] 6EE4478D: node started successfully
```

**Process logs** show the full PID:
```
2024-07-31 07:53:57 [info] <6EE4478D.0.1017>: processing request
```

With `IncludeName` enabled, the registered name appears:
```
2024-07-31 07:53:57 [info] <6EE4478D.0.1017> 'worker': processing request
```

With both `IncludeName` and `IncludeBehavior` enabled, the actor type appears:
```
2024-07-31 07:53:57 [info] <6EE4478D.0.1017> 'worker' main.MyWorker: processing request
```

**Meta process logs** show the alias:
```
2024-07-31 07:53:57 [info] Alias#<6EE4478D.123663.24065.0>: handling HTTP request
```

**Network logs** show local and remote node hashes:
```
2024-07-31 07:53:57 [info] 6EE4478D-90A29F11: connection established
```

These visual distinctions make scanning logs easier. At a glance, you can distinguish node events from process activity, meta process operations from network communications. The format itself tells you what layer of the system generated each message.

## Adding Context with Fields

Beyond the message text, you can attach structured fields - key-value pairs providing context. Fields enable correlation across log entries and make logs machine-parseable.

Consider a request handler. It receives a request with an ID. Every log entry related to that request should include the ID, allowing you to filter logs to just that request's activity:

```go
func (a *OrderProcessor) HandleMessage(from gen.PID, message any) error {
    order := message.(Order)

    a.Log().AddFields(
        gen.LogField{Name: "order_id", Value: order.ID},
        gen.LogField{Name: "customer_id", Value: order.CustomerID},
    )

    a.Log().Info("processing order")
    a.Log().Debug("validating payment")

    return nil
}
```

With `IncludeFields` enabled in the logger configuration, output shows:

```
2024-11-12 15:30:45 [info] <6EE4478D.0.1017>: processing order
                   fields order_id:12345 customer_id:67890
2024-11-12 15:30:45 [debug] <6EE4478D.0.1017>: validating payment
                    fields order_id:12345 customer_id:67890
```

Fields appear on a separate line below the message, prefixed with "fields" and aligned with the timestamp. Multiple fields are space-separated, each formatted as `key:value`. In JSON output, fields become separate JSON properties at the message's top level.

Fields only appear in output if the logger is configured to include them. The default logger requires `gen.NodeOptions.Log.DefaultLogger.IncludeFields = true`. Without this, fields are tracked internally but not displayed - useful if some loggers need fields while others don't.

Fields accumulate. Call `AddFields` multiple times and you add more fields rather than replacing existing ones. This supports incremental context building. Add session_id when the session starts. Add transaction_id when beginning a transaction. Add payment_id when processing payment. Each subsequent log includes all accumulated fields.

Remove fields with `DeleteFields`:

```go
a.Log().DeleteFields("order_id", "customer_id")
```

This clears the named fields from subsequent logs.

## Field Scoping

Field scoping handles nested contexts where you need temporary fields that shouldn't persist beyond a specific operation.

`PushFields` saves the current field set and starts a new scope. Add temporary fields, perform the operation (with those fields appearing in logs), then `PopFields` to restore the previous field set:

```go
a.Log().AddFields(gen.LogField{Name: "session_id", Value: "abc123"})

a.Log().PushFields()
a.Log().AddFields(gen.LogField{Name: "operation", Value: "payment"})
a.Log().Info("processing payment")

a.Log().PopFields()
a.Log().Info("payment complete")
```

Output shows:

```
2024-11-12 15:30:45 [info] <6EE4478D.0.1017>: processing payment
                   fields session_id:abc123 operation:payment
2024-11-12 15:30:45 [info] <6EE4478D.0.1017>: payment complete
                   fields session_id:abc123
```

The `operation` field exists only within the push/pop scope. After popping, logs include only `session_id`.

Scopes can nest. Each `PushFields` returns the stack depth. Each `PopFields` returns the new depth. This supports complex nested contexts - a request containing a transaction containing multiple operations, each adding its own contextual fields that disappear when the operation completes.

One restriction protects consistency: you can't delete fields while the field stack has active frames. If you've pushed fields, pop back to the base level before deleting. This prevents deleting a field that a pending pop might restore, which would leave the field state inconsistent.

## The Default Logger

Every node starts with a default logger writing to `os.Stdout`. Configure it through `gen.NodeOptions.Log.DefaultLogger`:

```go
options.Log.DefaultLogger.TimeFormat = time.DateTime
options.Log.DefaultLogger.IncludeBehavior = true
options.Log.DefaultLogger.IncludeName = true
options.Log.DefaultLogger.IncludeFields = true
```

`TimeFormat` controls timestamp display. Empty means nanoseconds since epoch. Any Go time format works - `time.DateTime`, `time.RFC3339`, or custom formats.

`IncludeBehavior` adds actor type names to process logs, showing which implementation generated each message.

`IncludeName` adds registered process names to process logs, making output more readable than PIDs alone.

`IncludeFields` controls whether structured fields appear in output.

`EnableJSON` switches to JSON format, with each message as a single-line JSON object.

To disable the default logger entirely, set `Disable: true`. Do this when using only custom loggers.

## Adding Custom Loggers

Custom loggers implement `gen.LoggerBehavior`:

```go
type LoggerBehavior interface {
    Log(message MessageLog)
    Terminate()
}
```

The `Log` method receives each message. The `Terminate` method handles cleanup when the logger is removed or the node shuts down.

Register a logger with `node.LoggerAdd`:

```go
node.LoggerAdd("errors", errorLogger, gen.LogLevelError, gen.LogLevelPanic)
```

The filter (final arguments) specifies which levels this logger handles. The logger receives only messages at those levels. Omit the filter to use `gen.DefaultLogFilter`, which includes all levels from Trace through Panic.

Loggers are stored per-level internally. Registering for Error and Panic stores the logger in both level maps. When an error occurs, the framework looks up the Error map and delivers the message to all loggers in that map.

Logger names must be unique. Reusing a name returns `gen.ErrTaken`. Remove a logger with `LoggerDelete` before adding a new one with the same name.

The `Log` method is called synchronously. If it blocks, it delays the logging path. For expensive operations - compressing logs, sending over network, database writes - make `Log` queue the work and return immediately, processing asynchronously.

## Process-Based Loggers

A process can act as a logger, receiving log messages through its mailbox. This integrates logging with the actor model.

Implement the `HandleLog` callback in your actor:

```go
type MyLogger struct {
    act.Actor
}

func (ml *MyLogger) HandleLog(message gen.MessageLog) error {
    switch m := message.Source.(type) {
    case gen.MessageLogNode:
        // Handle node log
    case gen.MessageLogProcess:
        // Process log - has PID, name, behavior
    case gen.MessageLogMeta:
        // Meta process log - has Alias
    case gen.MessageLogNetwork:
        // Network log - has local and remote nodes
    }
    return nil
}
```

Register the process as a logger:

```go
pid, err := node.Spawn(createMyLogger, gen.ProcessOptions{})
node.LoggerAddPID(pid, "mylogger", gen.LogLevelError, gen.LogLevelPanic)
```

Process-based logging queues messages asynchronously. The `Log` call places the message in the process's Log mailbox and triggers the process. The process handles log messages through `HandleLog`, processing them sequentially. The code that generated the log continues immediately without waiting.

This queuing prevents blocking. If the logger process is busy or the logging logic is expensive, messages queue and are processed when ready. The logging path stays fast.

Two details matter: First, a process can't add itself as a logger during `Init`. The process isn't registered yet, so `LoggerAddPID` fails. Spawn the logger, then add it. Second, when a logger process terminates, it's automatically removed from the logging system. No need to call `LoggerDeletePID` explicitly.

## Using Multiple Loggers

The fan-out architecture supports multiple loggers operating simultaneously with different purposes.

A typical production configuration disables the default logger and adds specialized loggers:

```go
options.Log.DefaultLogger.Disable = true

coloredLogger := colored.CreateLogger(colored.Options{})
node.LoggerAdd("console", coloredLogger,
    gen.LogLevelDebug, gen.LogLevelInfo, gen.LogLevelWarning,
    gen.LogLevelError, gen.LogLevelPanic)

rotateLogger := rotate.CreateLogger(rotate.Options{Path: "/var/log/myapp"})
node.LoggerAdd("file", rotateLogger)  // No filter = all levels
```

The colored logger handles debug through panic for console display during development. The rotate logger receives everything and writes to rotating files. Trace messages don't appear anywhere because no logger is registered for trace level.

Loggers can be added and removed dynamically. Start with console logging during development. Add file logging in staging. In production, remove console, keep files, add metrics forwarding. The system adapts without code changes.

## Controlling Verbosity

Different processes often need different verbosity. Most processes log at Info. Increase a troublesome process to Debug temporarily. Keep infrastructure processes at Warning to reduce noise.

```go
// Debugging a specific process
node.SetLogLevelProcess(suspiciousPID, gen.LogLevelDebug)

// Later, restore normal level
node.SetLogLevelProcess(suspiciousPID, gen.LogLevelInfo)
```

For processes generating high-volume logs, route them to a dedicated logger using a hidden logger. A trading engine logging every order would overwhelm general logs:

```go
// Register hidden logger for trading
tradingFileLogger := rotate.CreateLogger(rotate.Options{Path: "/var/log/trading"})
node.LoggerAdd(".trading", tradingFileLogger)

// Trading process uses only the hidden logger
tradingProcess.Log().SetLogger(".trading")
```

This creates isolation - the trading process logs only to `.trading`, and `.trading` receives only trading process logs. Other processes and loggers are unaffected. Without the hidden logger (using a regular logger name), the logger would also receive fan-out logs from all other processes.

Process-based loggers enable sophisticated handling. A logger process can aggregate metrics - count errors per minute, track which processes log most frequently. It can detect patterns - the same error repeating indicates a stuck condition. It can forward to external systems - send errors to Slack, metrics to Prometheus. As an actor, it maintains state, can be supervised for reliability, and integrates naturally with the rest of your system.

## Logger Implementations

The framework provides two logger implementations in separate packages for common needs:

**Colored** (`ergo.services/logger/colored`) - Terminal output with ANSI colors. Highlights Ergo types (PIDs, Atoms, Refs) and colorizes log levels (yellow for warnings, red for errors, etc.). Visual clarity for development, but has performance overhead. Not suitable for high-volume production logging.

**Rotate** (`ergo.services/logger/rotate`) - File logging with automatic rotation. Supports size-based and time-based rotation. Compresses old logs with gzip. Configurable retention policies. Production-ready for long-running systems generating substantial logs.

Both integrate with the logging system through `node.LoggerAdd`. You can combine them - colored for console during development, rotate for persistent storage, both receiving the same filtered log stream.

For implementation details and configuration options, see [Colored](../extra-library/loggers/colored.md) and [Rotate](../extra-library/loggers/rotate.md) in the extra library documentation.
