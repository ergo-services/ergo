# Actors

An extra library of actors implementations not included in the standard Ergo Framework library. This library contains packages with a narrow specialization. It also includes packages with external dependencies, as Ergo Framework adheres to a "zero dependency" policy.

## [Health](health.md)

Kubernetes health probe actor that serves `/health/live`, `/health/ready`, and `/health/startup` endpoints. Actors register named signals with probe type and optional heartbeat timeout. When a signal goes down, the corresponding probe endpoint returns 503.

**Use cases:** Kubernetes liveness/readiness/startup probes, container orchestration integration, dependency health tracking, graceful degradation.

## [Leader](leader.md)

Distributed leader election actor implementing Raft-inspired consensus algorithm. Provides coordination primitives for building systems that require single leader selection across a cluster.

**Use cases:** Task schedulers, resource managers, single-writer databases, distributed locks, cluster coordinators.

## [Metrics](metrics.md)

Prometheus metrics exporter actor that automatically collects and exposes Ergo node and network telemetry via HTTP endpoint. Provides observability for monitoring cluster health and resource usage.

**Use cases:** Production monitoring, performance analysis, capacity planning, debugging distributed systems.
