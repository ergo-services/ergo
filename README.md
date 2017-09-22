This project based on https://github.com/goerlang but everything has been merged
and refactored into this project and has no backward capability

# Node #

Implementation of Erlang/OTP node in Go

Supported features:

 * Publish listen port via EPMD
 * Handle incoming connection from other node using Erlang Distribution Protocol
 * Spawn Erlang-like processes
 * Register and unregister processes with simple atom
 * Send messages to registered (using atom) or not registered (using Pid) processes at Go-node or remote Erlang-node
 * Create own process with `GenServer` behaviour (like `gen_server` in Erlang/OTP)
 * Initiate connection to other node
 * RPC callbacks

Not supported:

 * Supervisors tree
 * Create own behaviours

## Example ##

See `examples/` to see simple implementation of node and `GenServer` process
