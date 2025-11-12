---
description: Handling synchronous requests asynchronously
---

# Async Request Handling

The actor model is asynchronous - processes send messages and continue without waiting. But real systems often need synchronous patterns. A client makes a request and waits for a response. An HTTP handler needs to return a result. A database query blocks until data arrives.

The challenge is handling these synchronous requirements without blocking the actor's message processing loop. Block on one request and you can't handle other messages. The actor becomes unresponsive.

This chapter explores patterns for handling synchronous-style requests while maintaining asynchronous actor behavior.
