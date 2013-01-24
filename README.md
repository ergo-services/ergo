# Go-node #

Implementation of Erlang/OTP node in Go

Supported features:

 * Publish listen port via EPMD
 * Handle incoming connection from other node using Erlang Distribution Protocol
 * Spawn Erlang-like processes
 * Register and unregister processes with simple atom
 * Send messages to registered (using atom) or not registered (using Pid) processes at Go-node or remote Erlang-node
 * Create own process with `GenServer` behaviour (like `gen_server` in Erlang/OTP)

Not supported (but should be):

 * Initiate connection to other node
 * Supervisors tree
 * Create own behaviours
 * RPC callbacks
 * Atom cache references to increase throughput

## Examples ##

See `examples/` to see example of go-node and `GenServer` process

Another project which uses this library: Eclus https://github.com/goerlang/eclus
