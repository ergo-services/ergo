# HTTP and Ergo framework

This is a simple application to demonstrate how you can combine stateless (HTTP) and stateful (actors) using Ergo Framework.

* app.go - declares Ergo application and controls handler_sup supervisor
* handler_sup.go - declares SimpleOneForOne supervisor (dynamic pool of workers). Every HTTP-request calls supervisor to start a new child process to handle this request
* handler.go - declares GenServer process to handle HTTP-requests.

You may also want to use another way to create a worker pool with a fixed number of worker-processes (use OneForOne for that)
