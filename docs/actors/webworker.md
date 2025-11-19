# WebWorker

WebWorker is a specialized actor for handling HTTP requests sent as `meta.MessageWebRequest` messages. It automatically routes requests to HTTP-method-specific callbacks and ensures the request completion signal is called.

Used with `meta.WebHandler` to convert HTTP requests into actor messages. See [Web](../meta-processes/web.md) for integration approaches.

## Purpose

When WebHandler sends `meta.MessageWebRequest` to an actor, that actor must:
1. Extract the message from mailbox
2. Determine HTTP method
3. Process the request
4. Write response to `http.ResponseWriter`
5. Call `Done()` to unblock the waiting HTTP handler

WebWorker automates this. Embed it, implement method-specific callbacks, and the framework handles routing and cleanup.

## Basic Usage

Embed `act.WebWorker` and implement callbacks for HTTP methods you handle:

```go
type APIWorker struct {
    act.WebWorker
}

func (w *APIWorker) HandleGet(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
    // Process GET request
    user := w.lookupUser(request.URL.Query().Get("id"))
    json.NewEncoder(writer).Encode(user)
    return nil
}

func (w *APIWorker) HandlePost(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
    // Process POST request
    var data CreateRequest
    json.NewDecoder(request.Body).Decode(&data)

    result := w.createResource(data)
    writer.WriteHeader(http.StatusCreated)
    json.NewEncoder(writer).Encode(result)
    return nil
}

func (w *APIWorker) HandleDelete(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
    id := request.URL.Query().Get("id")
    w.deleteResource(id)
    writer.WriteHeader(http.StatusNoContent)
    return nil
}
```

Spawn worker with registered name:

```go
type WebService struct {
    act.Actor
}

func (s *WebService) Init(args ...any) error {
    // Spawn worker
    _, err := s.SpawnRegister("api-worker",
        func() gen.ProcessBehavior { return &APIWorker{} },
        gen.ProcessOptions{},
    )
    if err != nil {
        return err
    }

    // Create WebHandler pointing to worker
    handler := meta.CreateWebHandler(meta.WebHandlerOptions{
        Worker: "api-worker",
    })

    _, err = s.SpawnMeta(handler, gen.MetaOptions{})
    // rest of setup...
}
```

When HTTP request arrives:
1. WebHandler sends `meta.MessageWebRequest` to "api-worker"
2. WebWorker detects message type, extracts HTTP method
3. WebWorker calls appropriate `Handle*` method
4. Your callback processes request, writes response
5. WebWorker calls `Done()` automatically
6. HTTP handler unblocks, response sent to client

## Available Callbacks

All callbacks are optional. Implement only the methods you need:

**HTTP methods**:
- `HandleGet(from gen.PID, writer http.ResponseWriter, request *http.Request) error`
- `HandlePost(from gen.PID, writer http.ResponseWriter, request *http.Request) error`
- `HandlePut(from gen.PID, writer http.ResponseWriter, request *http.Request) error`
- `HandlePatch(from gen.PID, writer http.ResponseWriter, request *http.Request) error`
- `HandleDelete(from gen.PID, writer http.ResponseWriter, request *http.Request) error`
- `HandleHead(from gen.PID, writer http.ResponseWriter, request *http.Request) error`
- `HandleOptions(from gen.PID, writer http.ResponseWriter, request *http.Request) error`

**Actor callbacks**:
- `Init(args ...any) error` - initialization
- `HandleMessage(from gen.PID, message any) error` - non-HTTP messages
- `HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)` - synchronous requests
- `HandleEvent(message gen.MessageEvent) error` - event subscriptions
- `Terminate(reason error)` - cleanup
- `HandleInspect(from gen.PID, item ...string) map[string]string` - introspection

Unimplemented HTTP methods return 501 Not Implemented automatically.

## Error Handling

Return `nil` to continue processing requests. Return non-nil error to terminate the worker:

```go
func (w *APIWorker) HandlePost(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
    var data CreateRequest
    if err := json.NewDecoder(request.Body).Decode(&data); err != nil {
        // Invalid JSON - return error to client, continue processing
        http.Error(writer, "Invalid JSON", http.StatusBadRequest)
        return nil
    }

    if err := w.createResource(data); err != nil {
        // Transient error - return error to client, continue processing
        http.Error(writer, "Create failed", http.StatusInternalServerError)
        return nil
    }

    writer.WriteHeader(http.StatusCreated)
    return nil
}
```

Returning error terminates the worker. Use this for fatal errors only (database connection lost, critical resource unavailable). For transient errors (validation, not found, conflict), write error response and return `nil`.

## Using with act.Pool

Single worker processes one request at a time. Use `act.Pool` for concurrent processing:

```go
type APIWorkerPool struct {
    act.Pool
}

func (p *APIWorkerPool) Init(args ...any) (act.PoolOptions, error) {
    return act.PoolOptions{
        PoolSize:          10,
        WorkerMailboxSize: 20,
        WorkerFactory:     func() gen.ProcessBehavior { return &APIWorker{} },
    }, nil
}
```

Spawn pool instead of single worker:

```go
_, err := s.SpawnRegister("api-worker",
    func() gen.ProcessBehavior { return &APIWorkerPool{} },
    gen.ProcessOptions{},
)
```

WebHandler sends requests to pool. Pool distributes across 10 workers. System handles 10 concurrent requests.

For details on pools, see [Pool](pool.md).

## Handling Non-HTTP Messages

WebWorker processes `meta.MessageWebRequest` specially, but also receives regular messages:

```go
func (w *APIWorker) HandleMessage(from gen.PID, message any) error {
    // meta.MessageWebRequest handled automatically by WebWorker
    // Other messages reach this callback
    switch m := message.(type) {
    case ConfigUpdate:
        w.config = m.Config
        w.Log().Info("Configuration updated")
    }
    return nil
}
```

This allows workers to receive configuration updates, control messages, or other actor communication while processing HTTP requests.

## Implementation Details

WebWorker implements `gen.ProcessBehavior` at low level. It manages the mailbox loop, detects `meta.MessageWebRequest`, routes by HTTP method, and calls `Done()` after processing.

The `Done()` call is critical. It cancels the context that WebHandler blocks on. Without it, HTTP request would timeout. WebWorker guarantees `Done()` is called even if your callback panics or returns error.

Default implementations for all callbacks exist. Unimplemented HTTP methods log warning and return 501 Not Implemented. This allows implementing only the methods you need without boilerplate for unsupported methods.
