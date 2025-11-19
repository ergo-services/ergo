# Colored Logger

The colored logger provides visual clarity for console output by applying color highlighting to log messages. Instead of monochrome text where errors blend with informational messages, each log level gets a distinct color, and framework types are highlighted automatically. This makes it easier to scan logs during development and debugging.

The logger writes directly to standard output with immediate formatting - no buffering, no delays. When a process logs a message, it appears instantly in your terminal with colors applied. This synchronous approach keeps logs simple and predictable during interactive development.

## Visual Organization

Color helps your eyes parse logs quickly. Log levels use consistent colors:

- **Trace** - Faint white (low importance, background noise)
- **Debug** - Magenta (development information)
- **Info** - White (normal operation)
- **Warning** - Yellow (attention needed)
- **Error** - Red bold (problems occurred)
- **Panic** - White on red background bold (critical failures)

Framework types also get color highlighting:

- **`gen.Atom`** - Green (names and identifiers)
- **`gen.PID`** - Blue (process identifiers)
- **`gen.ProcessID`** - Blue (named processes)
- **`gen.Ref`** - Cyan (references)
- **`gen.Alias`** - Cyan (meta-process identifiers)
- **`gen.Event`** - Cyan (event names)

When you log `process.Log().Info("started %s", pid)`, the PID renders in blue automatically. You don't annotate it - the logger detects the type and applies color. This works for any framework type used as an argument.

<figure><img src="../../.gitbook/assets/image (13).png" alt=""><figcaption></figcaption></figure>

## Log Format

Each log message follows a consistent structure:

```
<timestamp> <level> <source> [name] [behavior]: <message>
```

**Timestamp** appears first. By default, it's the Unix timestamp in nanoseconds. You can configure any format from Go's time package, or define your own. Nanosecond timestamps are sortable and precise, useful when correlating logs with traces or metrics.

**Level** shows the severity. The bracket format `[INFO]` or short form `[INF]` makes levels easy to grep. Color reinforces the level visually - you don't need to read the text to know something is an error.

**Source** identifies where the message originated:

- **Node logs** - Show the node name in green (CRC32 hash for compactness)
- **Network logs** - Show both local and peer node names
- **Process logs** - Show PID in blue, optionally the registered name in green, optionally the behavior type
- **Meta-process logs** - Show alias in cyan, optionally the behavior type

The optional components (name, behavior) are controlled by configuration. During development, you might want behavior names to understand which actor logged something. In production, you might omit them to reduce output.

**Message** is your formatted string with arguments. Framework types in arguments get color highlighting automatically.

## Configuration

The logger accepts several options during creation:

**TimeFormat** - Sets timestamp format. Any format from `time` package works (`time.RFC3339`, `time.Kitchen`, custom layouts). Leave empty for nanosecond timestamps. Nanoseconds are precise but hard to read. RFC3339 is human-friendly but verbose. Choose based on your use case.

**ShortLevelName** - Uses abbreviated level names: `[TRC]`, `[DBG]`, `[INF]`, `[WRN]`, `[ERR]`, `[PNC]`. Saves horizontal space in the terminal. Full names are clearer for people unfamiliar with the abbreviations.

**IncludeName** - Adds the registered process name to the source. If a process registers as `"worker"`, logs show the name in green next to the PID. Helpful when you have many processes and want to identify them by role rather than PID.

**IncludeBehavior** - Adds the behavior type name to the source. Logs show which actor implementation generated the message. Useful during development to understand code flow. In production, this adds noise if you have good message content.

**IncludeFields** - Includes structured logging fields in the output. Fields appear below the message with faint color. Useful when your log messages use context fields for correlation (request IDs, user IDs, etc.).

**DisableBanner** - Disables the Ergo logo banner on startup. The banner announces framework version and adds visual flair. Disable it in production or when running tests where the banner clutters output.

## Basic Usage

Register the colored logger in node options:

```go
package main

import (
    "ergo.services/ergo"
    "ergo.services/ergo/gen"
    "ergo.services/logger/colored"
)

func main() {
    logger := gen.Logger{
        Name:   "colored",
        Logger: colored.CreateLogger(colored.Options{}),
    }

    options := gen.NodeOptions{}
    options.Log.Loggers = []gen.Logger{logger}
    
    // Disable default logger to avoid duplicate output
    options.Log.DefaultLogger.Disable = true

    node, err := ergo.StartNode("demo@localhost", options)
    if err != nil {
        panic(err)
    }
    
    node.Log().Info("Node started with colored logger")
    node.Wait()
}
```

The default logger writes to stdout too, but without colors. If you don't disable it, you get each message twice - once colored, once plain. Disabling the default logger ensures only the colored version appears.

For detailed logger configuration options, see the `colored.Options` struct in the package. For understanding how loggers integrate with the framework, see [Logging](../../basics/logging.md).
