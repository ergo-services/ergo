# Boilerplate Code Generation&#x20;

The `ergo` tool allows you to generate the structure and source code for a project based on the Ergo Framework. To install it, use the following command:

`go install ergo.services/tools/ergo@latest`

Alternatively, you can build it from the source code available at [https://github.com/ergo-services/tools](https://github.com/ergo-services/tools).

When using `ergo` tool, you need to follow the specific template for providing arguments:

`Parent:Actor{param1:value1,param2:value2...}`

* **Parent** can be a _supervisor_ (specified earlier with `-with-sup`) or an _application_ (specified earlier with `-with-app`).
* **Actor** can be an _actor_ (added earlier with `-with-actor`) or a _supervisor_ (specified earlier with `-with-sup`).

This structured approach ensures the proper hierarchy and parameters are defined for your _actors_ and _supervisors_

### Available Arguments and Parameters :

* **`-init <node_name>`**: a required argument that sets the name of the node for your service. Available parameters:
  * **`tls`**: enables encryption for network connections (a self-signed certificate will be used).
  * **`module`**: allows you to specify the module name for the `go.mod` file.
* **`-path <path>`**: specifies the path for the code of the generated project.
* **`-with-actor <name>`**: adds an actor (based on `act.Actor`).
* **`-with-app <name>`**: adds an application. Available parameters:
  * **`mode`**: specifies the application's [start mode](../basics/application.md#application-startup-modes) (`temp` - Temporary, `perm` - Permanent, `trans` - Transient). The default mode is `trans`.\
    Example: `-with-app MyApp{mode:perm}`
* **`-with-sup <name>`**: adds a supervisor (based on `act.Supervisor`). Available parameters:
  * **`type`**: specifies the [type of supervisor](../actors/supervisor.md#supervisor-types) (`ofo` - One For One, `sofo` - Simple One For One, `afo` - All For One, `rfo` - Rest For One). The default type is `ofo`.
  * **`strategy`**: specifies the [restart strategy](../actors/supervisor.md#restart-strategy) for the supervisor (`temp` - Temporary, `perm` - Permanent, `trans` - Transient). The default strategy is `trans`.
* **`-with-pool <name>`**: adds a process pool actor (based on `act.Pool`). Available parameters:
  * **`size`**: Specifies the number of worker processes in the pool. By default, 3 processes are started.
* **`-with-web <name>`**: adds a Web server (based on `act.Pool` and `act.WebHandler`). Available parameters:
  * **`host`**: specifies the hostname for the Web server.
  * **`port`**: specifies the port number for the Web server. The default is `9090`.
  * **`tls`**: enables encryption for the Web server using the node's `CertManager`.
* **`-with-tcp <name>`**: adds a TCP server actor (based on `act.Actor` and `meta.TCP` meta-process). Available parameters:
  * **`host`**: specifies the hostname for the TCP server.
  * **`port`**: specifies the port number for the TCP server. The default is `7654`.
  * **`tls`**: enables encryption for the TCP server using the node's `CertManager`.
* **`-with-udp <name>`**: adds a UDP server actor (based on `act.Pool` , `meta.UDPServer` and `act.Actor` as worker processes). Available parameters:
  * **`host`**: specifies the hostname for the UDP server.
  * **`port`**: specifies the port number for the UDP server. The default is `7654`.
* **`-with-msg <name>`**: adds a message type for network interactions.
* **`-with-logger <name>`**: adds a logger from the extended library. Available loggers: [colored](../extra-library/loggers/colored.md), [rotate](../extra-library/loggers/rotate.md)
* **`-with-observer`**: adds the [Observer application](../extra-library/applications/observer.md).

### Example

For clarity, let's use all available arguments for `ergo` in the following example:&#x20;

<pre class="language-shell"><code class="lang-shell"><strong>$ ergo -path /tmp/project \
</strong><strong>      -init demo{tls} \
</strong><strong>      -with-app MyApp \
</strong><strong>      -with-actor MyApp:MyActorInApp \
</strong><strong>      -with-sup MyApp:MySup \
</strong><strong>      -with-actor MySup:MyActorInSup \
</strong><strong>      -with-tcp "MySup:MyTCP{port:12345,tls}" \
</strong><strong>      -with-udp MySup:MyUDP{port:54321} \
</strong><strong>      -with-pool MySup:MyPool{size:4} \
</strong><strong>      -with-web "MyWeb{port:8888,tls}" \
</strong><strong>      -with-msg MyMsg1 \
</strong><strong>      -with-msg MyMsg2 \
</strong><strong>      -with-logger colored \
</strong><strong>      -with-logger rotate \
</strong><strong>      -with-observer
</strong><strong>      
</strong>Generating project "/tmp/project/demo"...
   generating "/tmp/project/demo/apps/myapp/myactorinapp.go"
   generating "/tmp/project/demo/apps/myapp/myactorinsup.go"
   generating "/tmp/project/demo/cmd/myweb.go"
   generating "/tmp/project/demo/cmd/myweb_worker.go"
   generating "/tmp/project/demo/apps/myapp/mytcp.go"
   generating "/tmp/project/demo/apps/myapp/myudp.go"
   generating "/tmp/project/demo/apps/myapp/myudp_worker.go"
   generating "/tmp/project/demo/apps/myapp/mypool.go"
   generating "/tmp/project/demo/apps/myapp/mypool_worker.go"
   generating "/tmp/project/demo/apps/myapp/mysup.go"
   generating "/tmp/project/demo/apps/myapp/myapp.go"
   generating "/tmp/project/demo/types.go"
   generating "/tmp/project/demo/cmd/demo.go"
   generating "/tmp/project/demo/README.md"
   generating "/tmp/project/demo/go.mod"
   generating "/tmp/project/demo/go.sum"

Successfully completed.
</code></pre>

Pay attention to the values of the `-with-tcp` and `-with-web` arguments — they are enclosed in double quotes. If an argument has multiple parameters, they are separated by commas without spaces. However, since commas are argument delimiters for the shell interpreter, we enclose the entire value of the argument in double quotes to ensure the shell correctly processes the parameters.

In our example, we specified two loggers: `colored` and `rotate`. This allows for colored log messages in the standard output as well as logging to files with log rotation functionality. In this case, the default logger is disabled to prevent duplicate log messages from appearing on the standard output.

Additionally, we included the `observer` application. By default, this interface is accessible at `http://localhost:9911`.

As a result of the generation process, we have a well-structured project source code that is ready for execution:

```
 demo
├── apps
│  └── myapp
│     ├── myactorinapp.go
│     ├── myactorinsup.go
│     ├── myapp.go
│     ├── mypool.go
│     ├── mypool_worker.go
│     ├── mysup.go
│     ├── mytcp.go
│     ├── myudp.go
│     └── myudp_worker.go
├── cmd
│  ├── demo.go
│  ├── myweb.go
│  └── myweb_worker.go
├── go.mod
├── go.sum
├── README.md
└── types.go
```

The generated code is ready for compilation and execution:

<figure><img src="../.gitbook/assets/image (38).png" alt=""><figcaption></figcaption></figure>

Since this example includes the [observer application](../extra-library/applications/observer.md), you can open `http://localhost:9911` in your browser to access the web interface for [inspecting the node](observer.md) and its running processes.



