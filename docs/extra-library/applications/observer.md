# Observer

The Application _Observer_ provides a convenient web interface to view node status, network activity, and running processes in the node built with Ergo Framework. Additionally, it allows you to inspect the internal state of processes or meta-processes. The application is can also be used as a standalone tool Observer. For more details, see the section [Inspecting With Observer](../../tools/observer.md). You can add the _Observer_ application to your node during startup by including it in the node's startup options:

<pre class="language-go"><code class="lang-go">import (
	"ergo.services/ergo"
	"ergo.services/application/observer"
	"ergo.services/ergo/gen"
)

func main() {
	opt := gen.NodeOptions{
		Applications: []gen.ApplicationBehavior {
			observer.CreateApp(observer.Options{}),
		}
	}
<strong>	node, err := ergo.StartNode("example@localhost", opt)
</strong>	if err != nil {
		panic(err)
	}
	node.Wait()
}
</code></pre>

The function `observer.CreateApp` takes `observer.Options` as an argument, allowing you to configure the _Observer_ application. You can set:

* **Port**: The port number for the web server (default: `9911` if not specified).
* **Host**: The interface name (default: `localhost`).
* **LogLevel**: The logging level for the Observer application (useful for debugging). The default is `gen.LogLevelInfo`
