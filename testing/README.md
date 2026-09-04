# testing

Test support for the Ergo Framework. Four of these packages are tools for testing
your own actors and nodes; `tests` is the framework's own integration suite.

- `unit`: in-process harness for testing a single actor in isolation. It spawns the
  actor on a mock node and records every observable action (sends, calls, spawns,
  links, monitors, logs, terminations, cron jobs, node connections) so you can
  assert on them with a fluent builder. Reach for this first when testing actor
  logic.

- `mock`: standalone fakes for the `gen` interfaces (Node, Process, Connection,
  Core, CoreTargetManager, MetaProcess, Cron, Log, Network, RemoteNode, Registrar,
  Resolver). Each interface has a plain constructor and a recording one, plus a
  per-method override hook. Use it to drive a unit under test that talks to a `gen`
  interface directly, without standing up a full node.

- `check`: the shared assertion core. It defines the record types and the fluent
  matchers that `unit`, `mock` and `stage` all build on. Use it directly when you
  collect records yourself and want the same assertion grammar.

- `stage`: live multi-node harness. It starts real Ergo nodes in-process and lets
  them connect over the network, for end-to-end and distributed scenarios that a
  mock node cannot reproduce.

- `tests`: integration tests of the Ergo Framework itself (`local` for single-node,
  `distributed` for multi-node). This is the framework's own suite, not a tool for
  downstream projects.
