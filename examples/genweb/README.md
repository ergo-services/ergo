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

`webServer` implements gen.Web behavior and defines options for the HTTP server and HTTP handlers with two endpoints:
 * `/` - simple response with "Hello"
 * `/time/` - demonstrates async handling HTTP requests using `timeServer`

By default, it starts HTTP server on port 8080 so you can check it using your web-browser [http://localhost:8080/](http://localhost:8080/) or [http://localhost:8080/time/](http://localhost:8080/time/).

You may also want to benchmark this example on your hardware using the popular tool [wrk](https://github.com/wg/wrk). Here is the result of benchmarking on the AMD Ryzen Threadripper 3970X (64) @ 3.700GHz:

```
❯❯❯❯ wrk -t32 -c5000 --latency http://localhost:8080/
Running 10s test @ http://localhost:8080/
  32 threads and 5000 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency    14.44ms   20.62ms 389.13ms   88.66%
    Req/Sec    19.29k     2.85k   69.93k    84.94%
  Latency Distribution
     50%    6.93ms
     75%   19.59ms
     90%   37.81ms
     99%   98.17ms
  6149762 requests in 10.10s, 709.65MB read
Requests/sec: 608973.73
Transfer/sec:     70.27MB
```
