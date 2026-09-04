# Loggers

An extra library of logger implementations not included in the standard Ergo Framework library. This library contains packages with a narrow specialization. It also includes packages with external dependencies, as Ergo Framework adheres to a "zero dependency" policy.

## [Colored](colored.md)

Terminal output with ANSI colors. Highlights Ergo types (PIDs, Atoms, Refs) and colorizes log levels for visual clarity. Synchronous writes to stdout with immediate formatting.

**Use cases:** Local development, interactive debugging, fast visual scanning of logs in a terminal.

## [Rotate](rotate.md)

File logger with automatic time-based rotation and optional gzip compression. Asynchronous writes via background goroutine. Configurable retention policy.

**Use cases:** Production long-running services, time-windowed log archives, on-host log retention with bounded disk usage.

## [Sentry](sentry.md)

Forwards panics and errors to a Sentry project. Captures panic stack traces pointing at the panic origin and tags events by ergo subsystem (node, network, application, meta, process). Asynchronous, non-blocking.

**Use cases:** Centralized error tracking, panic alerting in production, grouping recurring failures by root cause.
