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
 * `/` - simple response with "Hello"
 * `/time/` - demonstrates async handling HTTP requests using `timeServer`

By default, it starts HTTP server on port 8080 so you can check it using your web-browser [http://localhost:8080/](http://localhost:8080/) or [http://localhost:8080/time/](http://localhost:8080/time/).

You may also want to benchmark this example on your hardware using the popular tool [wrk](https://github.com/wg/wrk):

`wrk -t32 -c5000 --latency http://localhost:8080/`
