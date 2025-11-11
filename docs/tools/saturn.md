---
description: Ergo Service Registry and Discovery
---

# Saturn - Central Registrar

`saturn` is a tool designed to simplify the management of clusters of nodes created using the Ergo Framework. It offers the following features:

* A unified registry for node registration within a cluster.
* The ability to manage multiple clusters simultaneously.
* The capability to manage the configuration of the entire cluster without restarting the nodes connected to Saturn (configuration changes are applied on the fly).
* Notifications to all cluster participants about changes in the status of applications running on nodes connected to Saturn.

The source code of the  `saturn` tool is available on the project's page: [https://github.com/ergo-services/tools](https://github.com/ergo-services/tools).

### Installation

To install `saturn`, use the following command:

```
$ go install ergo.services/tools/saturn@latest
```

Available arguments:

* **`host`**: Specifies the hostname to use for incoming connections.
* **`port`**: Port number for incoming connections. The default value is `4499`.
* **`path`**: Path to the configuration file `saturn.yaml`.
* **`debug`**: Enables debug mode for outputting detailed information.
* **`version`**: Displays the current version of Saturn.

### Starting Saturn

To start Saturn, a configuration file named `saturn.yaml` is required. By default, Saturn expects this file to be located in the current directory. You can specify a different location for the configuration file using the `-path` argument.

You can find an example configuration file in the project's Git repository.

#### Configuration file structure

The `saturn.yaml` configuration file contains two root elements:

1. **`Saturn`**: This section includes settings for the Saturn server.
   * You can configure the `Token` for access by remote nodes and specify certificate files for TLS connections.
   * By default, a _self-signed certificate_ is used. For clients to accept this certificate, they must enable the `InsecureSkipVerify` option when creating the client.
   * Changes to this section require a restart of the Saturn server.
2. **`Clusters`**: This section includes the configurations for clusters.
   * Changes in this section are automatically reloaded and sent to the registered nodes as updated configuration messages, without requiring a restart of Saturn.
   * The settings can target:
     * All nodes in all clusters.
     * Only nodes with a specified name in all clusters.
     * Only nodes within a specific cluster.
     * Only a node with a specified name within a specific cluster.

If the name of a configuration element ends with the suffix `.file`, the value of that element is treated as a file. The content of this file is then sent to the nodes as a `[]byte`.

To configure settings for all nodes in all clusters, use the `Clusters` section in the `saturn.yaml` configuration file. Here, you can define global settings that will apply to every node within every cluster managed by Saturn:

```yaml
Clusters:
    Var1: 123
    Var2: 12.3
    Var3: "123"
    Var4.file: "./myfile.txt"
    
    node@host.local:
        Var1: 456
```

in this example:

* `Var1`, `Var2`, `Var3`, and `Var4` will be applied to all nodes in all clusters.
* However, the value of `Var1` for nodes named `node@host.local` in any cluster will be overridden with the value `456`.

If nodes are registered without specifying a `Cluster` in `saturn.Options`, they become part of the _general cluster_. Configuration for the _general cluster_ should be provided in the `Cluster@` section

```yaml
Clusters:
    Var1: 123
    Cluster@:
        Var1: 789
        node@host:
            Var1: 456
```

In the example above:

* The variable `Var1` is set to `789` for the _general cluster_ (all nodes in the general cluster will receive `Var1: 789`).
* However, for the node `node@host.local` within the general cluster, `Var1` will be overridden to `456`.

Thus, all nodes in the _general cluster_ will inherit `Var1: 789`, except for `node@host.local`, which will specifically have `Var1: 456`. Other nodes in the _general cluster_ will retain the default values from the `Cluster@` section unless they are explicitly overridden in the configuration.

To specify settings for a particular cluster, use the element name `Cluster@<cluster name>` in the configuration file:

```yaml
Clusters:
    Var1: 123
    Cluster@mycluster:
        Var1: 321
        node@host: 654
```

### Service Discovery

Saturn can manage multiple clusters simultaneously, but resolve requests from nodes are handled only within their own cluster.

The name of a registered node must be unique within its cluster.

When a node registers, it informs the registrar which cluster it belongs to. Additionally, the node reports the applications running on it. Other nodes in the same cluster receive notifications about the newly connected node and its applications. Any changes in application statuses are also reported to the registrar, which in turn notifies all participants in the cluster.

For more details, see the [Saturn Client](../extra-library/registrars/saturn-client.md) section.&#x20;





