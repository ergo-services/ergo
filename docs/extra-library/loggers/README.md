# Loggers

Register a logger on a node to receive framework log records. These implementations are separate modules.

- [Colored](colored.md) formats records for a terminal.
- [Rotate](rotate.md) writes files and rotates them on a schedule.
- [Sentry](sentry.md) exports recovered panics and selected errors to Sentry.

See [Logging](../../basics/logging.md) for source records, filtering and registration.
