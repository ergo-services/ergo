# Port

{% hint style="info" %}
Introduced in 3.1.0 (not yet released. available in `v310` branch)
{% endhint %}

This implementation of the meta-process allows running external programs and communicating with them using _stdin_ and _stdout_. Communication can be performed in both text and binary formats.

To create a `meta.Port`, you need to use the `meta.CreatePort` function. It takes `meta.PortOptions` as an argument, which allows specifying the following options:

* `Cmd` command to run
* `Args` arguments for the command
* `Env` environment variables for the program being executed
* `EnableEnvMeta` adds environment variables of the meta-process
* `EnableEnvOS` adds OS environment variables
* `Tag` can help distinguish between multiple meta-processes running with the same `Cmd`/`Args`
* `Process` The name of the process that will handle `meta.MessagePort*` messages. If not specified, all messages will be sent to the parent process. You can also use an `act.Pool` process for distributing message handling
* `SplitFuncStdout` to specify a _split-function_. (uses `Scanner` from the standard `bufio` package) for processing data from _stdout_ in text mode. By default, data is split by lines
* `SplitFuncStderr` to specify a _split-function_ for the data from _stderr_
* `Binary` enables binary mode for the data from _stdout_. See [Binary mode](port.md#binary-mode) for more information

Below is an example of creating and running the external program `example-prog` using a meta-process `meta.Port`:

```go
type ActorPort struct {
	act.Actor
}

func (ap *ActorPort) Init(args ...any) error {
	var options meta.PortOptions

	options.Cmd = "example-prog"

	// create port
	metaport, err := meta.CreatePort(options)
	if err != nil {
		ap.Log().Error("unable to create Port: %s", err)
		return err
	}

	// spawn meta process
	id, err := ap.SpawnMeta(metaport, gen.MetaOptions{})
	if err != nil {
		ap.Log().Error("unable to spawn port meta-process: %s", err)
		return err
	}

	ap.Log().Info("started Port (iotxt) %s (meta-process: %s)", options.Cmd, id)
	return nil
}
```

### Binary mode

The implementation of `meta.Port` supports binary mode when working with _stdin_/_stdout_. To enable it, use the `meta.PortOptions.Binary.Enable` option and specify the binary format parameters:

* `ReadBufferSize` sets the buffer size for reading. The default size is 8192
* `ReadBufferPool` defines a pool for message buffers. The `Get` method should return an allocated buffer of type `[]byte`
* `ReadChunk` enables auto-chunking mode, which splits the incoming stream into chunks according to the specified parameters. For more details, see Auto chunking
* `WriteBufferKeepAlive` enables the software keep-alive mechanism – a specified value will be automatically sent at the interval defined in the option `WriteBufferKeepAlivePeriod`

When binary mode is enabled, the `SplitFuncStdout` option is ignored. However, _stderr_ is processed in text mode, and you can use the `SplitFuncStderr` option.

#### Auto chunking

This feature allows automatically chunking the incoming stream in binary mode. The chunks can be of fixed length – to achieve this, you need to specify the chunk size using the `FixedLength` option. For data with dynamic length, you must specify header parameters that will define the chunk length.

To enable the auto-chunking feature in binary mode, use the `meta.PortOptions.Binary.ReadChunk.Enable` option, and specify the parameters for automatic data chunking using the following options:

* `FixedLength` allows you to set the length of chunks in the _stdout_. In this case, all `Header*` options will be ignored. Leave this field with a zero value to work with dynamically sized chunks
* `HeaderSize` specifies the size of the header for dynamically sized chunks
* `HeaderLengthPosition` specifies the position in the header where the length is stored
* `HeaderLengthSize` specifies the size of the length in bytes (1, 2 or 4)
* `HeaderLengthIncludesHeader` specifies whether the length value includes the header size
* `MaxLength` allows you to set the maximum chunk length. If this value is exceeded, the meta-process will terminate, and the launched program will also be automatically stopped. Leave this value as zero if no length limits are required

For instance: incoming stream consists of dynamically sized chunks with a 7-byte header, and the length is encoded as a 32-bit number at the 3rd byte position (counting from 0). The length value includes the header size: _L = 7 + len(payload)_

```
  __ header _   ___ payload ___ 
 |           | |               |
 x x x L L L L . . . . . . . . .
```

In this case, the _auto-chunking_ options would be as follows

```go
...
var options meta.PortOptions
...
options.Binary.Enable = true
options.Binary.ReadChunk.Enable = true
options.Binary.ReadChunk.HeaderSize = 7
options.Binary.ReadChunk.HeaderLengthPosition = 3
options.Binary.ReadChunk.HeaderLengthSize = 4
options.Binary.ReadChunk.HeaderLengthIncludesHeader = true
```

You can find an example implementation in the repository [https://github.com/ergo-services/examples](https://github.com/ergo-services/examples), in the `port` project. Use the `-bin` argument to enable the demonstration of binary mode operation:

{% hint style="info" %}
available in `v310` branch of example repo  [https://github.com/ergo-services/examples](https://github.com/ergo-services/examples)
{% endhint %}

<figure><img src="../.gitbook/assets/image (39).png" alt=""><figcaption></figcaption></figure>
