# Colored

This package implements the `gen.LoggerBehavior` interface and provides the ability to output log messages to standard output with color highlighting.&#x20;

Below is a demonstration of log messages (from nodes, processes, and meta-processes) with different logging levels:

<figure><img src="../../.gitbook/assets/image (13).png" alt=""><figcaption></figcaption></figure>

### Format

`<time> <level> <log source> [process name] [process behavior]: <log message>`

When logging, the package also highlights in color the types `gen.Atom`, `gen.PID`, `gen.ProcessID`, `gen.Ref`, `gen.Alias`, `gen.Event`.

### Available options

Sets the format for the timestamp of log messages. You can use any existing format (see [time package](https://pkg.go.dev/time#pkg-constants)) or define your own. By default, the time is displayed in nanoseconds

* `ShortLevelName` Displays the shortened name of the log level
* `IncludeBehavior` Includes the name of the process behavior in the log message
* `IncludeName` includes the registered name of the process in the log message

### Example

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

	nopt := gen.NodeOptions{}
	nopt.Log.Loggers = []gen.Logger{logger}
	
	// disable default logger to get rid of duplicating log-messages
	nopt.Log.DefaultLogger.Disable = true

	node, err := ergo.StartNode("demo@localhost", nopt)
	if err != nil {
		panic(err)
	}
	node.Log().Warning("Hello World!!!")
	node.Wait()
}
```

{% hint style="info" %}
This package is not intended for use with intensive logging and may impact node performance. For log messages with the level `gen.LogLevelTrace`, color highlighting is applied only to the timestamp and the source of the log message; color highlighting is turned off for the body of the log message
{% endhint %}

