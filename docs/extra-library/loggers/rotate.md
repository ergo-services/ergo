# Rotate Logger

The rotate logger writes to an active file and rotates that file on a schedule. It can compress rotated files and remove older entries from the list it maintains during the current run.

The retention limit does not include files left by earlier runs. If disk usage must stay bounded across node restarts, configure an external cleanup policy as well.

## File Rotation Mechanics

Rotation happens based on time periods. You configure a duration - one minute, one hour, one day - and the logger creates a new file every period. The active file is always named `<Prefix>.log`. When the period ends, the logger:

1. Copies the active file to a timestamped filename: `<Prefix>.YYYYMMDDHHmi.log`
2. Optionally compresses it with gzip: `<Prefix>.YYYYMMDDHHmi.log.gz`
3. Truncates the active file to start fresh for the new period
4. Deletes old files if depth limit is configured

The active pathname remains `<Prefix>.log`; rotated copies carry timestamps. A reader following the open file descriptor may keep reading the old file after rotation. Configure the reader to follow the pathname and reopen it, for example with `tail -F` where that option is available.

The timestamp format is `YYYYMMDDHHmi` - year, month, day, hour, minute. This format sorts lexicographically, so `ls -l` shows files in chronological order. It's compact but human-readable.

## Asynchronous Writing

The logger uses an internal lock-free queue (MPSC - multi-producer single-consumer). When any process logs a message, it pushes to the queue and returns immediately. A single background goroutine pops messages from the queue and writes them to the file.

This design has several advantages:

**Queued writes** - `Log` submits the record to an unbounded queue instead of waiting for the file write. Slow disk I/O delays the writer and can grow the queue in memory. This separates actor logging from the disk wait; it does not provide bounded storage for a sustained backlog.

**Ordering** - Messages from a single producer maintain order. The queue preserves submission order, so logs reflect the actual sequence of events within each process.

**Batching** - The background goroutine processes messages continuously. If multiple messages arrive quickly, it writes them in a tight loop, reducing syscall overhead.

## Configuration

The logger requires a rotation period and accepts several optional parameters:

**Period** - The rotation interval. Minimum is `time.Minute`. Smaller periods create more files with less data each. Larger periods create fewer files with more data each. Choose based on how you analyze logs - if you search specific time ranges, shorter periods help. If you archive logs by day, use `24 * time.Hour`.

**Path** - Directory for log files. Defaults to `./logs` relative to the executable. The logger creates the directory if it doesn't exist. Supports `~` for home directory expansion (`~/logs` becomes `/home/user/logs`). Use absolute paths in production to avoid ambiguity.

**Prefix** - Filename prefix. Defaults to the executable name. The active file is `<Prefix>.log`, rotated files are `<Prefix>.YYYYMMDDHHmi.log[.gz]`. Use meaningful prefixes if multiple services log to the same directory.

**Compress** - Enables gzip compression for rotated files. The active file stays uncompressed and compressed copies have a `.log.gz` extension. The space saved depends on the log contents; compression adds work during rotation.

**Depth** - Limits the logger's in-memory list of files rotated during this run. Zero leaves that list unbounded; 24 retains up to 24 such files. Files from earlier runs are not counted. This setting alone does not bound total directory size across restarts.

**TimeFormat** - Timestamp format in log messages. Same as colored logger - any format from `time` package or custom layout. Empty string uses nanosecond timestamps. Choose based on readability vs. precision.

**IncludeName** - Includes registered process names in log messages. Helps identify which process logged what.

**IncludeBehavior** - Includes behavior type names in log messages. Useful during development to understand code flow.

**ShortLevelName** - Uses abbreviated level names (`[TRC]`, `[DBG]`, etc.) instead of full names. Saves space in log files.

## Basic Usage

Configure the rotate logger in node options:

```go
package main

import (
    "time"

    "ergo.services/ergo"
    "ergo.services/ergo/gen"
    "ergo.services/logger/rotate"
)

func main() {
    options := rotate.Options{
        Period:   time.Hour,
        Path:     "/var/log/myapp",
        Prefix:   "myapp",
        Compress: true,
        Depth:    24,
    }

    logger, err := rotate.CreateLogger(options)
    if err != nil {
        panic(err)
    }

    nodeOpts := gen.NodeOptions{
        Log: gen.LogOptions{
            Loggers: []gen.Logger{
                {Name: "rotate", Logger: logger},
            },
        },
    }

    node, err := ergo.StartNode("demo@localhost", nodeOpts)
    if err != nil {
        panic(err)
    }

    node.Log().Info("Node started, logging to /var/log/myapp/myapp.log")
    node.Wait()
}
```

This configuration:
- Rotates every hour
- Stores logs in `/var/log/myapp/`
- Names files `myapp.log` (active) and `myapp.202411191200.log.gz` (rotated with compression)
- Keeps the last 24 files **this process rotated**

`Depth` is not retention by age, and not by what is in the directory. The logger keeps an in-memory list of the files it has rotated during its own lifetime and removes the oldest only once that list grows past `Depth`. It never scans the directory at startup, so files left by an earlier run are neither counted nor deleted - restart the node every hour with `Depth: 24` and the directory grows without bound. To bound disk use across restarts, do it outside the logger: logrotate, a cron sweep, or a volume with its own retention.

For detailed logger configuration options, see the `rotate.Options` struct in the package. For understanding how loggers integrate with the framework, see [Logging](../../basics/logging.md).
