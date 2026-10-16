# Registrars

A registrar resolves node addresses and, where supported, application routes and cluster configuration. The core includes an embedded host-level registrar; these clients are separate modules.

- [etcd Client](etcd-client.md) uses etcd registrations, leases and prefix watches.
- [Saturn Client](saturn-client.md) connects to a dedicated Saturn server and receives updates over its persistent connection.

Choose according to the discovery and configuration services your deployment needs. [Service Discovering](../../networking/service-discovering.md) describes the common interface and differences in scope.
