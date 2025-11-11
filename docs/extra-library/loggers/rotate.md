# Rotate

This package implements the `gen.LoggerBehavior` interface and provides the capability to log messages to a file, with support for log file rotation at a specified interval.

### Available options

* `Period` Specifies the rotation period (minimum value: `time.Minute`)
* `TimeFormat` Sets the format for the timestamp in log messages. You can choose any existing format (see [time package](https://pkg.go.dev/time#pkg-constants)) or define your own. By default, timestamps are in nanoseconds
* `IncludeBehavior` includes the process behavior in the log
* `IncludeName` includes the registered process name
* `ShortLevelName` enables shortnames for the log levels
* `Path` directory for the log files, default: `./log`
* `Prefix` defines the log files name prefix (`<Path>/<Prefix>.YYYYMMDDHHMi.log[.gz]`)
* `Compress` Enables gzip compression for log files
* `Depth` Specifies the number of log files in rotation

### Example

```go
package main

import (
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/gen"
	"ergo.services/logger/rotate"
)

func main() {
	var options gen.NodeOptions
	ropt := rotate.Options{Period: time.Minute, Compress: false}
	rlog, err := rotate.CreateLogger(ropt)
	if err != nil {
		panic(err)
	}
	logger := gen.Logger{
		Name:   "rotate",
		Logger: rlog,
	}
	options.Log.Loggers = append(options.Log.Loggers, logger)

	node, err := ergo.StartNode("demo@localhost", options)
	if err != nil {
		panic(err)
	}
	node.Wait()
}
```



