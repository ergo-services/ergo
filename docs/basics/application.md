# Application

In Ergo Framework, an _Application_ is a component that allows you to group a set of [actors](../actors/actor.md) (and/or [supervisors](../actors/supervisor.md)) and manage them as a single entity. The _Application_ is responsible for starting each actor within the group and handling dependencies on other _Applications_ if specified in the configuration. Additionally, the _Application_ monitors the running processes and, depending on the mode chosen during startup, may stop all group members and itself if one of the processes terminates for any reason.

### Create Application

To create an _Application_ in Ergo Framework, the `gen.ApplicationBehavior` interface is provided:

```go
type ApplicationBehavior interface {
	// Load invoked on loading application using method ApplicationLoad 
	// of gen.Node interface.
	Load(node Node, args ...any) (ApplicationSpec, error)
	// Start invoked once the application started
	Start(mode ApplicationMode)
	// Terminate invoked once the application stopped
	Terminate(reason error)
}
```

All methods of the `gen.ApplicationBehavior` interface must be implemented.&#x20;

For example, if you want to combine an actor `A1` and a supervisor `S1` into a single application, the code for creating such an _Application_ would look like this:

```go
func createApp() gen.ApplicationBehavior {
	return &app{}
}

type app struct{}

func (a *app) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name: "my_app",
		Group: []gen.ApplicationMemberSpec{
			{
				Name:    "a1", // start with registered name
				Factory: factory_A1,
			},
			{
				// omitted field Name. will be started with no name
				Factory: factory_S1,
			},
		},
	}, nil

}
func (a *app) Start(mode gen.ApplicationMode) {} // no need? keep it empty
func (a *app) Terminate(reason error)         {}

...
myApp := createApp()
appName, err := node.ApplicationLoad(myApp)
```

Since an _Application_, by its nature, is not a process, its startup is divided into two stages: _loading_ and _starting_.

To load the application, the `gen.Node` interface provides the method `ApplicationLoad`. You pass your implementation of the `ApplicationBehavior` interface to this method. The node will then call the `Load` callback method of the `gen.ApplicationBehavior` interface, which must return the application's specification as a `gen.ApplicationSpec`:

```go
type ApplicationSpec struct {
	Name        Atom
	Description string
	Version     Version
	Depends     ApplicationDepends
	Env         map[Env]any
	Group       []ApplicationMemberSpec
	Mode        ApplicationMode
	Weight      int
	LogLevel    LogLevel
}

```

In the `gen.ApplicationSpec` structure, the `Name` field defines the name under which your application will be registered. This name must be unique within the node. However, the registry for application names and the registry for process names are separate. Although it is possible to run a process with a name identical to the application name, this could lead to confusion, so care should be taken when choosing names for both processes and applications. The `Description` field is optional.

The `Group` field contains a group of specifications for launching processes, defined by `gen.ApplicationMemberSpec`. If the `Name` field in `gen.ApplicationMemberSpec` is not empty, the process will be launched using the `SpawnRegister`method of the `gen.Node` interface. The processes in the group are launched sequentially, following the order specified in the `Group` field.

The `Env` field is used to set environment variables for all processes started within the application. Environment variables are inherited in the following order: _Node > Application > Parent > Process_.

If your application depends on other applications or network services, you can specify these dependencies using the `Depends` field. These dependencies are taken into account when starting the application.

The `Mode` field defines the startup mode for the application and its behavior if one of the processes in the group stops. This value serves as the default mode when the application is started using the `ApplicationStart(...)` method of the `gen.Node` interface. However, the startup mode can be overridden using the methods `ApplicationStartTransient`, `ApplicationStartTemporary`, or `ApplicationStartPermanent` of the `gen.Node` interface.

Finally, the `LogLevel` field allows you to set the logging level for the entire group of processes. If this option is not specified, the node's logging level will be used. However, individual processes can override this setting by specifying it in the `ApplicationMemberSpec.Options` during the process startup specification.

### Application Startup Modes

Ergo Framework provides three startup modes for applications, each determining how the application behaves when one of its processes stops:

1. **`gen.ApplicationModeTemporary`**:\
   This is the default startup mode if no specific mode is indicated in the application's specification (`gen.ApplicationSpec`). In this mode, the termination of any individual process (for any reason) does not lead to the shutdown of the entire application. The application will only stop when all the processes in the application group, as defined in the `Group` field of the `gen.ApplicationSpec`, have stopped. The application will always stop with the reason `gen.TerminationReasonNormal`.
2. **`gen.ApplicationModeTransient`**:\
   The application stops only when one of its processes terminates unexpectedly (with a reason other than `gen.TerminationReasonNormal` or `gen.TerminationReasonShutdown`). In such cases, all running processes will receive an exit signal with the reason `gen.TerminationReasonShutdown`. Once all processes have stopped, the application will terminate with the reason that caused the initial process failure. If all processes complete normally, the application will stop with the reason `gen.TerminationReasonNormal`.
3. **`gen.ApplicationModePermanent`**:\
   In this mode, the termination of any process in the application, for any reason, will trigger the shutdown of the entire application. All running processes will receive an exit signal with the reason `gen.TerminationReasonShutdown`. Once all processes have stopped, the application will terminate with the reason that caused the shutdown. If all processes complete without errors, the application will stop with the reason `gen.TerminationReasonNormal`.

Processes that are part of the application group can launch child processes. These child processes will inherit the application's attributes, but their termination does not affect the application's logic; their role is informational. You can determine whether a process belongs to an application using the `Info` method of the `gen.Process` interface or the `ProcessInfo` method of the `gen.Node` interface. Both methods return a `gen.ProcessInfo` structure, which contains the `Application` field indicating the process's application affiliation.

### Starting an Application

In Ergo Framework, the `gen.Node` interface provides several methods to start an application:

* **`ApplicationStart`**: Starts the loaded application in the mode specified by `gen.ApplicationSpec.Mode`.
* **`ApplicationStartTemporary`**: Starts the application in `gen.ApplicationModeTemporary`, regardless of the mode specified in the application's specification.
* **`ApplicationStartTransient`**: Starts the application in `gen.ApplicationModeTransient`, regardless of the mode specified in the application's specification.
* **`ApplicationStartPermanent`**: Starts the application in `gen.ApplicationModePermanent`, regardless of the mode specified in the application's specification.

Before starting the application's processes, the node checks the dependencies specified in `gen.ApplicationSpec.Depends`. If the application depends on other applications, the node will attempt to start them first. Therefore, all dependent applications must either already be loaded or running before starting the application. If the dependencies are not met, the method will return the error `gen.ErrApplicationDepends`.

Once all dependencies are satisfied, the node starts launching the processes according to the order specified in `gen.ApplicationSpec.Group`. If any process fails to start, all previously started processes will be forcibly stopped using the `Kill` method of the `gen.Node` interface, and the startup process will return an error.

After all processes are successfully started, the `Start` callback method of the `gen.ApplicationBehavior` interface is called, and the application transitions to the _running_ state.

You can retrieve information about the application using the `ApplicationInfo` method of the `gen.Node` interface. This method returns a `gen.ApplicationInfo` structure that contains summary information about the application and its current state.

### Stopping an Application

To stop an application, the `gen.Node` interface provides the `ApplicationStop` method. When this method is called, the application sends an exit signal to all of its processes with the reason `gen.TerminateReasonShutdown` and waits for them to stop completely.

If the processes do not stop within 5 seconds, a timeout error will be returned. If the `ApplicationStop` method is called again while the application is still waiting for its processes to stop, it will return the error `gen.ErrApplicationStopping`.

For forcefully stopping the application's processes, the `ApplicationStopForce` method is available in the `gen.Node`interface. In this case, all of the application's processes will be forcibly terminated using the `Kill` method of the `gen.Node`interface.

Additionally, the `gen.Node` interface provides the `ApplicationStopWithTimeout` method, which allows you to specify a custom timeout period for the application to stop.

Once all of the application's processes have stopped, the node will call the `Stop` method of the `gen.ApplicationBehavior` interface, completing the shutdown process.
