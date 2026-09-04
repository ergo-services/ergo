# Sentry Logger

The sentry logger forwards panics and errors to a Sentry project, complete with stack traces pointing at the panic origin. When a production node recovers a panic in an actor callback, that event is usually written to a file and noticed hours later, if at all. Sentry-backed logging surfaces the same event in your issue tracker within seconds, grouped by panic value, with the originating frames marked as application code.

The logger does not replace your console or file logger. It runs alongside them and captures a narrow slice of the log stream: every panic from anywhere in the framework, and errors from the higher-level subsystems (node, network, application). Meta processes and individual actors are off by default to keep noisy actor-level errors from drowning your Sentry quota, and can be turned on per source when needed.

## What Gets Sent

Two severities are forwarded; the others stay local. The matrix maps source against level:

|         | Node | Network | Application | Meta | Process |
|---------|:----:|:-------:|:-----------:|:----:|:-------:|
| Panic   |  X   |    X    |      X      |  X   |    X    |
| Error   |  X   |    X    |      X      | opt  |   opt   |

Every panic event reaches Sentry regardless of which subsystem produced it. The framework recovers panics in actor callbacks, supervisors, pools, web workers, meta processes, applications, cron jobs and network handlers; each recovery site formats a `Log().Panic()` message which the logger sees and captures.

Error-level events from the node, network and application subsystems are forwarded by default. These are the operational layer: a network handshake failure, an application that refuses to start, a node configuration error. They tend to be infrequent and worth a Sentry issue every time.

Error-level events from meta processes and individual actors are opt-in via `CaptureMetaErrors` and `CaptureProcessErrors`. A misbehaving actor can produce many errors per second, and you usually want to investigate those in your local logs first rather than pay for ingest in Sentry. Turn the flag on when you have specific signals you want to track centrally.

Lower levels (warning, info, debug, trace) are never forwarded. Sentry is for things that need attention, not a general log sink.

## Stack Traces at Panics

For panic events the logger captures the goroutine stack at the moment the panic was recovered. The captured stack points at the line of code that actually panicked rather than at the framework's recovery wrapper. In Sentry this appears as an `Exception` with `Type: "panic"`, value set to the recovered panic value, and a stack trace with caller-first ordering.

Frames are marked as application code by default; frames from the Go runtime, the Sentry SDK and `ergo.services/*` are tagged as framework code so Sentry's UI collapses them out of the way and highlights the lines that belong to your project.

Sentry groups events by the `Exception.Value`. Since that value is the recovered panic message (`nil pointer dereference`, `index out of range`, your own panic string), every reoccurrence of the same root cause ends up in the same issue rather than scattered by the surrounding format string.

If you wrap `Log().Panic()` in a helper layer of your own, increase `SkipFrames` so the captured stack still trims your wrapper out and starts where the panic actually originated.

## Tagging and Context

Every event carries a `source` tag identifying which subsystem produced it: `node`, `network`, `application`, `meta` or `process`. Additional tags depend on the source: node and network events include the node name (and peer name for network); application events include the application name and run mode; meta and process events include their identifier (alias or PID) and optionally the behavior type.

Structured fields attached via `Log().AddFields()` arrive in Sentry as `Extra` data. The same fields you use to correlate logs locally show up in the Sentry event panel, so you can filter or pivot on `request_id`, `user_id`, or any other context you have already wired through your logging path.

## Non-Blocking Delivery

The `Log` method is called synchronously by the framework. To avoid holding up the logging path, the sentry logger accepts the message and hands it off to a background worker that builds and ships the Sentry envelope. The framework continues without waiting for network I/O to Sentry.

When the internal queue is full, new events are dropped silently. The queue cap exists to bound memory under a panic storm; on a well-behaved system it stays empty most of the time. If you see drops in practice, raise `QueueLimit` or investigate the actor that keeps panicking.

`Terminate()` is given a bounded amount of time to drain the queue and flush events that the Sentry SDK has already buffered for transport. After that window the logger gives up and the node exits.

## Isolation

The logger creates its own Sentry client and hub. It does not call `sentry.Init()` and does not touch the global `sentry.CurrentHub()`. If you already use the Sentry SDK directly elsewhere in your process, the integrations do not interfere with each other.

## Configuration

The logger accepts the following options:

**DSN** - Sentry project DSN. Leave empty to fall back to the `SENTRY_DSN` environment variable as handled by the Sentry SDK. Required either inline or via environment.

**Environment** - Tag attached to every event. Common values are `production`, `staging`, `development`. Sentry uses this to filter and segment issues.

**Release** - Version of the running binary. When set, Sentry can link an issue to the deploy that introduced it and surface regressions across releases.

**ServerName** - Override for the auto-detected hostname. Useful when running in containers where the default hostname is meaningless.

**CaptureMetaErrors** - Forwards error-level events from meta processes. Off by default. Turn on when meta-process errors are operationally relevant.

**CaptureProcessErrors** - Forwards error-level events from individual actors. Off by default. Turn on selectively; an actor in a bad state can produce many errors per second.

**QueueLimit** - Cap on the internal event queue. Events past the cap are dropped. The default is conservative; raise it if you expect panic storms and want to keep more events in flight rather than drop them.

**FlushTimeout** - Time budget for `Terminate()` to drain the queue and flush the SDK's transport buffer before the logger returns. The default is enough for normal shutdown; reduce it if your node has very strict shutdown deadlines.

**SkipFrames** - Top frames trimmed from captured panic stacks. The default matches the standard call chain. Increase if you wrap `Log().Panic()` in your own helpers, decrease if you call into the logger from a place closer to the panic.

**BeforeSend** - Hook forwarded to the Sentry SDK. Runs for every outgoing event. Return `nil` to drop, return the (possibly modified) event to forward. Useful for scrubbing sensitive fields or applying additional filters.

## Basic Usage

Register the sentry logger alongside your console or file logger in node options:

```go
package main

import (
    "ergo.services/ergo"
    "ergo.services/ergo/gen"
    "ergo.services/logger/sentry"
)

func main() {
    sl, err := sentry.CreateLogger(sentry.Options{
        DSN:         "https://<key>@sentry.io/<project>",
        Environment: "production",
        Release:     "myapp@1.2.3",
    })
    if err != nil {
        panic(err)
    }

    options := gen.NodeOptions{}
    options.Log.Loggers = []gen.Logger{
        {Name: "sentry", Logger: sl},
    }

    node, err := ergo.StartNode("demo@localhost", options)
    if err != nil {
        panic(err)
    }

    node.Wait()
}
```

The default logger continues to write to stdout; the sentry logger receives the same stream and forwards the subset described above. To limit Sentry to just errors and panics without producing log work for the rest of the levels, register the logger with an explicit level filter:

```go
node.LoggerAdd("sentry", sl, gen.LogLevelError, gen.LogLevelPanic)
```

For detailed logger configuration options, see the `sentry.Options` struct in the package. For understanding how loggers integrate with the framework, see [Logging](../../basics/logging.md).
