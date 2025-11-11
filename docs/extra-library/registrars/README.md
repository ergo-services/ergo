# Registrars

An extra library of registrars or client implementations not included in the standard Ergo Framework library. This library contains packages with a narrow specialization. It also includes packages with external dependencies, as Ergo Framework follows a "zero dependency" policy.

## Available Registrars

### [Saturn Client](saturn-client.md)

A client library for the central [Saturn](../../tools/saturn.md) registrar. Provides service discovery, configuration management, and real-time cluster event notifications through a centralized registrar service.

**Features:**

* Centralized service discovery
* Real-time event notifications
* Configuration management
* TLS security support
* Token-based authentication

### [etcd Client](etcd-client.md)

A client library for [etcd](https://etcd.io/), a distributed key-value store. Provides decentralized service discovery, hierarchical configuration management with type conversion, and automatic lease management.

**Features:**

* Distributed service discovery
* Hierarchical configuration with type conversion from strings (`"int:123"`, `"float:3.14"`)
* Automatic lease management and cleanup
* Real-time cluster change notifications
* TLS/authentication support
* Four-level configuration priority system

Choose Saturn for centralized management with a dedicated registrar service, or etcd for a distributed approach with built-in consensus and reliability guarantees.
