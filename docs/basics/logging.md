# Logging

In Ergo Framework, a flexible logging system is implemented that allows multiple loggers to exist within a node. These loggers can be configured to receive specific logging levels.

For logging purposes, the `gen.Node` and `gen.Process` interfaces provide the `Log` method. This method returns the `gen.Log` interface, which is used for logging messages within the framework.

```go
type Log interface {
	Level() LogLevel
	SetLevel(level LogLevel) error

	Logger() string
	SetLogger(name string)

	Trace(format string, args ...any)
	Debug(format string, args ...any)
	Info(format string, args ...any)
	Warning(format string, args ...any)
	Error(format string, args ...any)
	Panic(format string, args ...any)
}

```

Using the `gen.Log` interface, you can manage the logging level not only for the node but also for each individual process (including meta-processes). There are six standard logging levels that can be set using the `SetLevel` method:

* `gen.LogLevelDebug`
* `gen.LogLevelInfo`
* `gen.LogLevelWarning`
* `gen.LogLevelError`
* `gen.LogLevelPanic`
* `gen.LogLevelDisabled`

Additionally, there are specialized logging levels:

* **`gen.LogLevelDefault`**: This is the default logging level. When a node starts with this level, it defaults to `gen.LogLevelInfo`. For applications or processes, the logging level inherits from the node. Meta-processes inherit the logging level of their parent process.
* **`gen.LogLevelTrace`**: This level is used exclusively for deep debugging. To avoid accidental activation, this level cannot be set through the `SetLevel` method of the `gen.Log` interface. Instead, it can only be enabled during the startup of a node or process through `gen.NodeOptions.Log.Level` or `gen.ProcessOptions.LogLevel`.

By default, the logging level for the node is set to `gen.LogLevelInfo`. Processes inherit the node's logging level at startup unless explicitly set in `gen.ProcessOptions`.

The `SetLogger` method allows you to restrict logging to a specified logger. If no logger is specified, the log message will be delivered to all registered loggers. You can retrieve the list of registered loggers using the `Loggers` method of the `gen.Node` interface.

The `gen.LogLevelDisabled` level can be used to temporarily disable logging, either for the entire node or a specific process.

Furthermore, the `gen.Node` interface provides two methods for process-specific logging control:

* `ProcessLogLevel(pid gen.PID)`: Retrieves the current logging level of a specific process.
* `SetProcessLogLevel(pid gen.PID, level gen.LogLevel)`: Sets the desired logging level for a specific process.

### Logger

By default, a node in Ergo Framework uses a standard logger, and its parameters can be configured during node startup via `gen.NodeOptions.Log.DefaultLogger`. You have the option to disable the default logger and use a process-based logger or integrate one (or more) loggers from an additional logging library.

If you wish to create your own logger, you simply need to implement the `gen.LoggerBehavior` interface. This interface defines the behavior of a logger, allowing you to customize the logging system according to your application's requirements. By implementing this interface, you can control how log messages are processed, formatted, and routed, providing flexibility beyond the default logging system.&#x20;

```go
type LoggerBehavior interface {
	Log(message MessageLog)
	Terminate()
}
```

To add a new logger, the `gen.Node` interface provides two methods:

* `LoggerAdd(name string, logger gen.LoggerBehavior, filter ...gen.LogLevel)`: This method allows you to add a logger (an object implementing the `gen.LoggerBehavior` interface) to the node's logging system.
* `LoggerAddPID(pid gen.PID, name string, filter ...gen.LogLevel)`: This method adds a process as a logger. In this case, the node sends log messages to the specified process. The `act.Actor` class provides a callback method `HandleLog` for handling such log messages.

Both methods accept a `filter` argument, which allows you to specify a set of log levels that the logger should handle. For example, if you want your logger to only receive messages at the `gen.LogLevelPanic` and `gen.LogLevelInfo` levels, you can list them in the filter:

```
node.LoggerAdd("my logger", myLogger, gen.LogLevelPanic, gen.LogLevelInfo)
```

If the `filter` argument is not specified, the default set of log levels, `gen.DefaultLogFilter`, will be used:

<pre class="language-go"><code class="lang-go">[]gen.LogLevel{
    LogLevelTrace,
    LogLevelDebug,
    LogLevelInfo,
    LogLevelWarning,
<strong>    LogLevelError,
</strong>    LogLevelPanic,
}
</code></pre>

The logging methods of the `gen.LoggerBehavior` interface will only be invoked for messages that match the specified logging levels.

Reusing a logger name is not allowed. If a logger with the same name already exists, the `LoggerAdd` or `LoggerAddPID`function will return the error `gen.ErrTaken`. To resolve this, you must first remove the existing logger from the system using the appropriate method: `LoggerDelete` or `LoggerDeletePID` from the `gen.Node` interface.

When a logger is removed from the system, the `Terminate` callback method of the `gen.LoggerBehavior` interface is called.

{% hint style="info" %}
The methods of the `gen.LoggerBehavior` interface are called synchronously by the node or process. If you are implementing your own logger, it is important to keep this in mind and avoid blocking these calls with complex or time-consuming log message processing.

To mitigate this issue, it is recommended to consider using a [process-based logger.](logging.md#process-logger) In this case, all log messages are placed in the process's mailbox and processed asynchronously, ensuring that the `gen.LoggerBehavior` interface methods are not blocked by the logging logic. This approach allows for more efficient handling of log messages without impacting the overall performance of the node or process.
{% endhint %}

### Default logger

When starting a node, you can specify logging parameters using the `gen.NodeOptions.Log.DefaultLogger` option. These parameters will be applied to the default logger, which outputs all log messages in text format to a specified output (by default, `os.Stdout`). With these options, you can configure:

*   **`Output`**: The interface used for log message output. To direct log messages to both standard output and a file simultaneously, you can use the `io.MultiWriter` function from Golang's standard library:

    ```go
    var options gen.NodeOptions
    logFile, _ := os.Create("out.log")
    options.Log.Logger.Output = io.MultiWriter(os.Stdout, logFile)
    ```
* **`TimeFormat`**: Defines the format for the timestamp of log messages. You can use any existing format, such as `time.DateTime` (see [time package constants](https://pkg.go.dev/time#pkg-constants)), or define your own custom format. By default, the timestamp is output in nanoseconds.
* **`IncludeBehavior`**: Adds the behavior of the process or meta-process to the log message.
* **`IncludeName`**: Adds the registered name of the process to the log message, if it exists.
* **`Filter`**: Specifies the logging levels that the default logger will handle. Leave this field empty to use the default set of levels (`gen.DefaultLogFilter`).
* **`Disable`**: Disables the default logger.

Here is an example of how the standard logger's output might look with these options applied:

```go
TimeFormat: time.DateTime
IncludeBehavior: true
IncludeName: true
```

```go
func (a *MyActor) Init(args ...any) error {
    ...
    a.Log().Info("starting my actor")
    return nil
}
...
pid, err := node.SpawnRegister("example", factoryMyActor, gen.ProcessOptions{})
node.Log().Info("started MyActor with PID %s", pid)
...
2024-07-31 07:53:57 [info] <6EE4478D.0.1017> 'example' main.MyActor: starting my actor
2024-07-31 07:53:57 [info] 6EE4478D: started MyActor with PID <6EE4478D.0.1017>
```

The first field is the timestamp of the log message, the second field is the log level, and the third field is the source of the message. The sources can come from various interfaces:

* **`gen.Node.Log()`**: Uses the node's name in the form of a CRC32 hash. Example: `9F35C982`
* **`gen.Process.Log()`**: The string representation of the process identifier `gen.PID`. Example: `<9F35C982.0.1013>`
* **`gen.MetaProcess.Log()`**: The string representation of the meta-process identifier `gen.Alias`. Example: `Alias#<9F35C982.123663.24065.0>`
* **`gen.Connection.Log()`**: String representations of the local and remote nodes. Example: `9F35C982-90A29F11`

### Process-logger

If you want to manage log message streams using the familiar actor model, any process in a running node can serve as a logger. In the `act.Actor` implementation, received log messages are passed as the `message` argument to the `HandleLog` callback method.

To add your actor to the node as a logger, use the `LoggerAddPID` method from the `gen.Node` interface:

```go
func factoryMyLogger() gen.ProcessBehavior {
    return &MyLogger{}
}

type MyLogger struct {
    act.Actor
}

func (ml *MyLogger) HandleLog(message gen.MessageLog) error {
    switch m := message.Source.(type) {
    case gen.MessageLogNode:
        // handle message 
    case gen.MessageLogProcess:
        // handle message
    case gen.MessageLogMeta:
        // handle message
    case gen.MessageLogNetwork:
        // handle message
    }
    ...
    return nil
}
...
pid, err := node.Spawn(factoryMyLogger, gen.ProcessOptions{})
node.LoggerAddPID(pid)
```

Note that the `LoggerAddPID` method of the `gen.Node` interface is not available to a process during its initialization state, as the node has not yet recognized the process (see the Process section). Therefore, an actor cannot add itself as a logger within the `Init` callback method of the `act.ActorBehavior` interface.&#x20;

When a logger process terminates, it will be automatically removed from the node's logging system.

### Implementations of gen.Logger

To extend the capabilities of the Ergo Framework, two additional loggers have been implemented:

* [**colored**](../extra-library/loggers/colored.md): Enables colored highlighting for log messages in the console.
* [**rotate**](../extra-library/loggers/rotate.md): Allows logging to a file with support for log file rotation at specified intervals.

You can disable the default logger and add both of these loggers, enabling colored output in the console while also saving logs to a file with rotation. You can find an example of using them in the `demo` project [here](https://github.com/ergo-services/examples).
