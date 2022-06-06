## Web demo scenario ##

This example implements a simple application that starts two child processes - webServer and timeServer.

```
webNode (node.Node) main.go  -> webApp (gen.Application) app.go
                         |
                          -> webServer (gen.Web) web.go
                         |       |
                         |        ->> url '/' handler (gen.WebHandler) web_root_handler.go
                         |        ->> url '/time/' handler (gen.WebHandler) web_time_handler.go
                         |
                          -> timeServer (gen.Server) time.go
```

`webServer` implements gen.Web behavior and defines options for the HTTP server and dynamic pool of HTTP handlers. There are two HTTP endpoints:
 * '/' - simple response with "Hello"
 * '/time' - demonstrates async handling HTTP requests using `timeServer`
