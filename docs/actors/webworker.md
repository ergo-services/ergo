---
description: To handle HTTP-requests
---

# WebWorker

The `act.WebWorker` actor implements the low-level `gen.ProcessBehavior` interface and allows HTTP requests to be handled as asynchronous messages. It is designed to be used with a web server (refer to the [Web](../meta-processes/web.md) section in meta-processes). To launch a process based on `act.WebWorker`, you need to embed it in your object and implement the required methods from the `act.WebWorkerBehavior` interface.

Example:

```go
type MyWebWorker struct {
    act.WebWorker
}

func factoryMyWebWorker() gen.ProcessBehavior {
    return &MyWebWorker{}
}
```

To work with your object, `act.WebWorker` uses the `act.WebWorkerBehavior` interface. This interface defines a set of callback methods:

```go
type WebWorkerBehavior interface {
	gen.ProcessBehavior

	// Init invoked on a spawn WebWorker for the initializing.
	Init(args ...any) error

	// HandleMessage invoked if WebWorker received a message sent with gen.Process.Send(...).
	// Non-nil value of the returning error will cause termination of this process.
	// To stop this process normally, return gen.TerminateReasonNormal
	// or any other for abnormal termination.
	HandleMessage(from gen.PID, message any) error

	// HandleCall invoked if WebWorker got a synchronous request made with gen.Process.Call(...).
	// Return nil as a result to handle this request asynchronously and
	// to provide the result later using the gen.Process.SendResponse(...) method.
	HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)

	// Terminate invoked on a termination process
	Terminate(reason error)

	// HandleEvent invoked on an event message if this process got subscribed on
	// this event using gen.Process.LinkEvent or gen.Process.MonitorEvent
	HandleEvent(message gen.MessageEvent) error

	// HandleInspect invoked on the request made with gen.Process.Inspect(...)
	HandleInspect(from gen.PID, item ...string) map[string]string

	// HandleGet invoked on a GET request
	HandleGet(from gen.PID, writer http.ResponseWriter, request *http.Request) error
	// HandlePOST invoked on a POST request
	HandlePost(from gen.PID, writer http.ResponseWriter, request *http.Request) error
	// HandlePut invoked on a PUT request
	HandlePut(from gen.PID, writer http.ResponseWriter, request *http.Request) error
	// HandlePatch invoked on a PATCH request
	HandlePatch(from gen.PID, writer http.ResponseWriter, request *http.Request) error
	// HandleDelete invoked on a DELETE request
	HandleDelete(from gen.PID, writer http.ResponseWriter, request *http.Request) error
	// HandleHead invoked on a HEAD request
	HandleHead(from gen.PID, writer http.ResponseWriter, request *http.Request) error
	// HandleOptions invoked on an OPTIONS request
	HandleOptions(from gen.PID, writer http.ResponseWriter, request *http.Request) error
}

```

All methods in the `act.WebWorkerBehavior` interface are optional for implementation.

It is most efficient to use `act.WebWorker` in combination with `act.Pool` for load balancing when handling HTTP requests:

```go
//
// WebWorker
//
func factory_MyWebWorker() gen.ProcessBehavior {
	return &MyWebWorker{}
}

type MyWebWorker struct {
	act.WebWorker
}

// Handle GET requests. 
func (w *MyWebWorker) HandleGet(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
	w.Log().Info("got HTTP request %q", request.URL.Path)
	w.WriteHeader(http.StatusOK)
	return nil
}

//
// Pool of workers
//
type MyPool struct {
	act.Pool
}

func factory_MyPool() gen.ProcessBehavior {
	return &MyPool{}
}

// Init invoked on a spawn Pool for the initializing.
func (p *MyPool) Init(args ...any) (act.PoolOptions, error) {
	opts := act.PoolOptions{
		WorkerFactory: factory_MyWebWorker,
	}
	p.Log().Info("started process pool of MyWebWorker with %d workers", opts.PoolSize)
	return opts, nil
}
```

An example implementation of a web server using `act.WebWorker` and `act.Pool` for load distribution when handling HTTP requests can be found in the repository at [https://github.com/ergo-services/examples](https://github.com/ergo-services/examples), the project `demo`.

<figure><img src="../.gitbook/assets/image (9).png" alt=""><figcaption></figcaption></figure>
