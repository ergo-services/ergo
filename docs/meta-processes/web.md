# Web

To handle synchronous HTTP requests with the actor model in Ergo Framework, they must be converted into asynchronous messages. For this purpose, two meta-processes have been created: `meta.WebServer` and `meta.WebHandler`. The first is used to start an HTTP server, and the second transforms synchronous HTTP requests into messages that are sent to a worker process. A worker process can be created using `act.WebWorker`, which processes the incoming messages asynchronously.

### Creating and Spawning meta.WebHandler&#x20;

You can create a `meta.WebHandler` using the `meta.CreateWebHandler` function. This function creates an object that implements the `gen.MetaProcessBehavior` and `http.Handler` interfaces.

It takes `meta.WebHandlerOptions` as an argument:

* `Process`: The name of the process to which transformed asynchronous `meta.MessageWebRequest` messages are sent. If not specified, these messages are sent to the parent meta-process.
* `RequestTimeout`: The time allowed for the process to handle the message, with a default of 5 seconds.

After successful creation, you need to start the meta-process using `SpawnMeta` from the `gen.Process` interface.&#x20;

Once the meta-process is running, it can be used as an HTTP request handler

```go
func factory_MyWeb() gen.ProcessBehavior {
	return &MyWeb{}
}

type MyWeb struct {
	act.Actor
}

// Init invoked on a start this process.
func (w *MyWeb) Init(args ...any) error {
	// create an HTTP request multiplexer
	mux := http.NewServeMux()

	// create a root handler meta-process
	root := meta.CreateWebHandler(meta.WebHandlerOptions{
		// use process based on act.WebWorker 
		// and spawned with registered name "mywebworker"
		Worker: "mywebworker",
	})
	
	// spawn this meta-process
	rootid, err := w.SpawnMeta(root, gen.MetaOptions{})
	if err != nil {
		w.Log().Error("unable to spawn WebHandler meta-process: %s", err)
		return err
	}
	
	// add our meta-process as a handler of HTTP-requests to the mux
	// since it implements http.Handler interface
	mux.Handle("/", root)
	
	// you can also use your middleware function:
	// mux.Handle("/", middleware(root))
	
	w.Log().Info("started WebHandler to serve '/' (meta-process: %s)", rootid)

	// create and spawn web server meta-process
	// with the mux we created to handle HTTP-requests
	//
	// see below...
	...
	return nil
}

```

### Creating and Spawning meta.WebServer

To create the `meta.WebServer` meta-process, use the `meta.CreateWebServer` function with the `meta.WebServerOptions` argument. These options allow you to set:

* `Host`: The interface on which the port will be opened for handling HTTP requests.
* `Port`: The port number.
* `CertManager`: Enables TLS encryption for the HTTP server. You can use the node's `CertManager` to activate the node's certificate by using the `CertManager()` method of the `gen.Node` interface.
* `Handler`: Specifies the HTTP request handler.

When the meta-process is created, the HTTP server starts. If the server fails to start, `meta.CreateWebServer` returns an error. After successful creation, start the meta-process using `SpawnMeta(...)` from the `gen.Process` interface.

Example:

```go
type MyWeb struct {
	act.Actor
}

// Init invoked on a start this process.
func (w *MyWeb) Init(args ...any) error {
	// create an HTTP request multiplexer
	mux := http.NewServeMux()
	
	// create and spawn your handler meta-processes
	// and add them to the mux
	//
	// see above
	...

	// create and spawn web server meta-process
	serverOptions := meta.WebServerOptions{
		Port: 9090,
		Host: "localhost",
		// use node's certificate if it was enabled there
		CertManager: w.Node().CertManager(),
		Handler:     mux,
	}

	webserver, err := meta.CreateWebServer(serverOptions)
	if err != nil {
		w.Log().Error("unable to create Web server meta-process: %s", err)
		return err
	}
	webserverid, err := w.SpawnMeta(webserver, gen.MetaOptions{})
	if err != nil {
		// invoke Terminate to close listening socket
		webserver.Terminate(err)
	}

	w.Log().Info("started Web server %s: use http[s]://%s:%d/", 
			webserverid, 
			serverOptions.Host, 
			serverOptions.Port)
	return nil
}

```

Example can be found in the repository at [https://github.com/ergo-services/examples](https://github.com/ergo-services/examples), specifically in the `demo` project.

<figure><img src="../.gitbook/assets/image (9).png" alt=""><figcaption></figcaption></figure>

