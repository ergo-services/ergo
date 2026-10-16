# Actors

These actors are distributed as separate modules. Each can be embedded to implement application-specific callbacks.

- [Health](health.md) combines process signals into HTTP liveness, readiness and startup probes.
- [Leader](leader.md) elects a coordinator within the membership view maintained by each node. Read its partition and fencing constraints before using it for exclusive work.
- [Metrics](metrics.md) collects framework measurements and exports Prometheus metrics, with hooks for application measurements.
