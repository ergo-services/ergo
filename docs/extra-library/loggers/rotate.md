# Rotate Logger

The rotate logger writes log messages to files with automatic rotation based on time intervals. Instead of a single growing log file that eventually fills the disk, the logger creates new files periodically and optionally compresses old ones. This keeps disk usage predictable and makes log files manageable for analysis and archival.

The logger operates asynchronously - log messages enter a queue and a background goroutine writes them to the file. This design prevents blocking your processes when disk I/O is slow. Logging happens in the background while your actors continue processing messages without waiting for disk writes to complete.

## File Rotation Mechanics

Rotation happens based on time periods. You configure a duration - one minute, one hour, one day - and the logger creates a new file every period. The active file is always named `<Prefix>.log`. When the period ends, the logger:

1. Copies the active file to a timestamped filename: `<Prefix>.YYYYMMDDHHmi.log`
2. Optionally compresses it with gzip: `<Prefix>.YYYYMMDDHHmi.log.gz`
3. Truncates the active file to start fresh for the new period
4. Deletes old files if depth limit is configured

This approach ensures the active file always has the same name. You can tail it (`tail -f <Prefix>.log`) and it works across rotations. The timestamped copies accumulate in the log directory, creating a chronological archive.

The timestamp format is `YYYYMMDDHHmi` - year, month, day, hour, minute. This format sorts lexicographically, so `ls -l` shows files in chronological order. It's compact but human-readable.

## Asynchronous Writing

The logger uses an internal lock-free queue (MPSC - multi-producer single-consumer). When any process logs a message, it pushes to the queue and returns immediately. A single background goroutine pops messages from the queue and writes them to the file.

This design has several advantages:

**Non-blocking** - Logging never blocks your process. If the disk is slow or the file system stalls, your actors continue running. The queue absorbs bursts of messages.

**Ordering** - Messages from a single producer maintain order. The queue preserves submission order, so logs reflect the actual sequence of events within each process.

**Batching** - The background goroutine processes messages continuously. If multiple messages arrive quickly, it writes them in a tight loop, reducing syscall overhead.

## Configuration

The logger requires a rotation period and accepts several optional parameters:

**Period** - The rotation interval. Minimum is `time.Minute`. Smaller periods create more files with less data each. Larger periods create fewer files with more data each. Choose based on how you analyze logs - if you search specific time ranges, shorter periods help. If you archive logs by day, use `24 * time.Hour`.

**Path** - Directory for log files. Defaults to `./logs` relative to the executable. The logger creates the directory if it doesn't exist. Supports `~` for home directory expansion (`~/logs` becomes `/home/user/logs`). Use absolute paths in production to avoid ambiguity.

**Prefix** - Filename prefix. Defaults to the executable name. The active file is `<Prefix>.log`, rotated files are `<Prefix>.YYYYMMDDHHmi.log[.gz]`. Use meaningful prefixes if multiple services log to the same directory.

**Compress** - Enables gzip compression for rotated files. The active file stays uncompressed for fast writing. When rotating, the logger compresses the copy, reducing disk usage by 5-10x for text logs. Compressed files have `.log.gz` extension. Use compression if disk space matters more than CPU for compression.

**Depth** - Limits the number of retained log files. When rotating, if the number of files exceeds Depth, the logger deletes the oldest file. Set to 0 (default) for unlimited retention. Set to a specific number (e.g., 24) to keep the last 24 periods. This prevents unbounded disk usage.

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
- Keeps last 24 hourly files (24 hours of history)
- Deletes files older than 24 hours automatically

For detailed logger configuration options, see the `rotate.Options` struct in the package. For understanding how loggers integrate with the framework, see [Logging](../../basics/logging.md).
