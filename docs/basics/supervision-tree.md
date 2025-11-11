---
description: process control
---

# Supervision Tree

To create fault-tolerant applications, Ergo Framework introduces a process structuring model. The core idea of this model is to divide processes into two types:

* worker
* supervisor

Worker processes perform computations, while supervisors are responsible solely for managing worker processes.

In Ergo Framework, worker processes are actors based on [`act.Actor`](../actors/actor.md). Supervisors, on the other hand, are actors based on [`act.Supervisor`](../actors/supervisor.md). The role of `act.Supervisor` is to start child processes and restart them according to the chosen restart strategy. Several [restart strategies](../actors/supervisor.md#restart-strategy) are available for this purpose. A child process can be not only an actor based on `act.Actor` but also a supervisor based on `act.Supervisor`. This allows you to form a hierarchical structure for managing processes. Thanks to this approach, your solution becomes more reliable and fault-tolerant.







&#x20;
