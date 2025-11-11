# Supervisor

A _supervisor_ in Ergo Framework is a specialized actor responsible for managing child processes. Its main tasks include starting child processes, monitoring their life cycles, and restarting them if necessary according to the chosen [restart strategy](supervisor.md#restart-strategy).&#x20;

`act.Supervisor` implements the low-level `gen.ProcessBehavior` interface. Similar to `act.Actor`, it has the embedded `gen.Process` interface. This means that an object based on `act.Supervisor` has access to all the methods provided by the `gen.Process` interface.

To create a _supervisor_ actor, you simply embed `act.Supervisor` into your object. Additionally, you need to create a factory function for the supervisor. Here is an example:

```go
type MySupervisor struct {
    act.Supervisor
}

func factoryMySupervisor() gen.ProcessBehavior {
    return &MySupervisor{}
}
```

To work with your object, `act.Supervisor` uses the `act.SupervisorBehavior` interface. This interface defines a set of callback methods for initialization, handling asynchronous messages, and handling synchronous calls:

```go
type SupervisorBehavior interface {
	gen.ProcessBehavior

	// Init invoked on a spawn Supervisor process. 
	// This is a mandatory callback for the implementation
	Init(args ...any) (SupervisorSpec, error)

	// HandleChildStart invoked on a successful child process starting if option EnableHandleChild
	// was enabled in gen.SupervisorSpec
	HandleChildStart(name gen.Atom, pid gen.PID) error

	// HandleChildTerminate invoked on a child process termination 
	// if option EnableHandleChild was enabled in gen.SupervisorSpec
	HandleChildTerminate(name gen.Atom, pid gen.PID, reason error) error

	// HandleMessage invoked if Supervisor received a message sent with gen.Process.Send(...).
	// Non-nil value of the returning error will cause termination of this process.
	// To stop this process normally, return gen.TerminateReasonNormal or
	// gen.TerminateReasonShutdown. Any other - for abnormal termination.
	HandleMessage(from gen.PID, message any) error

	// HandleCall invoked if Supervisor got a synchronous request made with gen.Process.Call(...).
	// Return nil as a result to handle this request asynchronously and
	// to provide the result later using the gen.Process.SendResponse(...) method.
	HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)

	// Terminate invoked on a termination supervisor process
	Terminate(reason error)

	// HandleInspect invoked on the request made with gen.Process.Inspect(...)
	HandleInspect(from gen.PID) map[string]string
}

```

The `Init` method is mandatory for implementation, while the other methods are optional. This method returns the supervisor specification `act.SupervisorSpec`, which includes the following options:

* **Type**: The type of _supervisor_ (`act.SupervisorType`).
* **Children**: The list of child process specifications (`[]act.SupervisorChildSpec`). This parameter also determines the order of child process startups.
* **Restart**: Defines the restart strategy for child processes.
* **EnableHandleChild**: Enables the invocation of the callback methods `HandleChildStart` and `HandleChildStop` during the startup and shutdown of child processes.
* **DisableAutoShutdown**: Prevents the _supervisor_ from stopping if all child processes terminate non-faultily.

Here is an example implementation of the `act.SupervisorBehavior` interface for the _supervisor_ `MySupervisor` with one child process `MyActor`:

```go
//
// Child process implementation
//

func factoryMyActor() gen.ProcessBehavior {
    return &MyActor{}
}

type MyActor struct {
    act.Actor
}

func (a *MyActor) Init(args ...any) error {
    a.Log().Info("starting child process MyActor")
    return nil
}

//
// gen.SupervisorBehavior implementation
//

func (s *MySupervisor) Init(args ...any) (act.SupervisorSpec, error) {
    s.Log().Info("initialize supervisor...")
    spec := act.SupervisorSpec{
        Children: []act.SupervisorChildSpec{
            act.SupervisorChildSpec{
                Name: "myChild",
                Factory: factoryMyActor,
            },
        }
    }
    return spec, nil
}
...
pid, err := node.Spawn(factoryMySupervisor, gen.ProcessOptions{})
node.Log().Info("MySupervisor is started succesfully with pid %s", pid)
...


```



### Supervisor Types

The type of _supervisor_ in the `SupervisorSpec` defines its behavior. In Ergo Framework, several _supervisor_ types are implemented:

#### act.SupervisorTypeOneForOne&#x20;

This is the default _supervisor_ type. If the type is not specified in the `SupervisorSpec`, this one will be used. Upon startup, the _supervisor_ sequentially starts child processes as specified in `act.SupervisorSpec.Children`. If an error occurs while starting the child processes, the _supervisor_ process will terminate.

Each child process is launched with the associated name defined in the `Name` option of the child process specification `act.SupervisorChildSpec`. This means that only one process can be started for each child process specification. If this field is empty, the _supervisor_ will use the `Spawn` method from the `gen.Process` interface. Otherwise, the `SpawnRegister` method will be used.

Stopping a child process triggers the restart strategy for that process. Depending on the restart strategy defined by `act.SupervisorSpec.Restart.Strategy`, the process will either be restarted or stopped. The [restart strategy](supervisor.md#restart-strategy) does not affect other processes.

The child process is restarted with the same set of arguments that were used in its previous startup.

The `KeepOrder` option in `act.SupervisorSpec.Restart` is ignored for this _supervisor_ type. When a new child process specification is added via the `AddChild` method (provided by `act.Supervisor`), the _supervisor_ automatically starts the new child process. When a child specification is disabled (using method) `DisableChild`, the _supervisor_ stops the corresponding child process with the reason `gen.TerminateReasonShutdown`. The _supervisor_ will automatically terminate if no child processes remain (with the reasons `gen.TerminateReasonNormal` or `gen.TerminateReasonShutdown`). You can enable the `DisableAutoShutdown` option in `act.SupervisorSpec` to prevent this.

#### act.SupervisorTypeOneForAll

When starting, this _supervisor_ sequentially launches child processes according to the list of child process specifications. If an error occurs during the startup of child processes, the _supervisor_ process terminates.

A child process is started with the associated name defined in the `Name` option of `act.SupervisorChildSpec`.

Stopping a child process in this _supervisor_ triggers the [restart strategy](supervisor.md#restart-strategy) for all child processes according to `act.SupervisorSpec.Restart.Strategy`.

If the `KeepOrder` option is enabled, child processes are stopped in reverse order. Without `KeepOrder`, they are stopped simultaneously. After stopping, the _supervisor_ restarts them in the specified order.

Each child process is restarted with the same arguments as in the previous run.

When a new child process specification is added (using `AddChild`), the _supervisor_ automatically starts the child process. When disabling (`DisableChild`), the _supervisor_ stops the corresponding child process with the reason `gen.TerminateReasonShutdown`.

The `DisableAutoShutdown` option allows the _supervisor_ to continue running even if all child processes have stopped (terminated with `gen.TerminateReasonNormal` or `gen.TerminateReasonShutdown`).

#### act.SupervisorTypeRestForOne

When starting, this _supervisor_ sequentially launches child processes according to the list of child process specifications. If an error occurs during the startup of child processes, the _supervisor_ process terminates.

A child process is started with the associated name defined in the `Name` option of `act.SupervisorChildSpec`.

When a child process terminates, the [restart strategy](supervisor.md#restart-strategy) is activated. All child processes starting from the last one (according to the order in `act.SupervisorSpec.Children`) to the process that triggered the restart are stopped.

With the `KeepOrder` option enabled, child processes are stopped sequentially in reverse order. If the option is disabled, child processes stop simultaneously.

After all child processes are stopped, the _supervisor_ restarts them according to the order specified in `act.SupervisorSpec.Children`.

Each child process is restarted with the same set of arguments used in the previous startup.

Each child process is launched with the associated name defined in the `Name` option of the child process specification `act.SupervisorChildSpec`.&#x20;

#### act.SupervisorTypeSimpleOneForOne

This type of _supervisor_ is a simplified version of `act.SupervisorTypeOneForOne`. It does not start child processes on startup. Instead, child processes are started using the `StartChild` method.

As in `act.SupervisorTypeOneForOne`, the termination of a child process triggers the [restart strategy](supervisor.md#restart-strategy) for that process. Depending on the strategy defined in `act.SupervisorSpec.Restart.Strategy`, the process will either be restarted or stopped. The restart strategy does not affect other processes.

Child processes in this _supervisor_ are launched without an associated name, and multiple processes can be started from one child process specification.

The `KeepOrder` option in `act.SupervisorSpec.Restart` is ignored for this _supervisor_ type.

The `DisableAutoShutdown` option is also ignored, and stopping all child processes does not result in the termination of the _supervisor_ process.

### Child Process Specification

This specification describes the parameters for starting a child process. This is done using `act.SupervisorChildSpec`:

* **Name**: The specification's name, used as the associated name for the process (except for `act.SupervisorTypeSimpleOneForOne`).
* **Factory, Options, Args**: Parameters needed to launch a new process.
* **Significant**: Determines the importance of the child process. The termination of a significant process may lead to the _supervisor_ and all child processes stopping, depending on the restart strategy:
  * `act.SupervisorStrategyTemporary`: Termination of a significant child process leads to the termination of the _supervisor_ and all children.
  * `act.SupervisorStrategyTransient`: Non-crash termination (e.g., `gen.TerminateReasonNormal` or `gen.TerminateReasonShutdown`) of a significant child process leads to the termination of the _supervisor_ and all children, while a crash triggers the restart strategy.
  * `act.SupervisorStrategyPermanent`: The `Significant` field is ignored, and termination always triggers the restart strategy.

Additionally, the `Significant` field is ignored in _supervisors_ of types `act.SupervisorTypeOneForOne` and `act.SupervisorSimpleOneForOne`.

### Restart Strategy

The `Restart` option in `act.SupervisorSpec` is of type `act.SupervisorRestart` and defines parameters for the restart strategy:

* **Strategy**: Specifies the restart strategy type:
  * `act.SupervisorStrategyTransient`: A crash triggers a restart. If the process terminates normally (e.g., `gen.TerminateReasonNormal` or `gen.TerminateReasonShutdown`), the restart strategy is not activated. This is the default strategy.
  * `act.SupervisorStrategyTemporary`: No restart occurs, even in the event of a crash.
  * `act.SupervisorStrategyPermanent`: Any termination triggers a restart, ignoring `DisableAutoShutdown`.
* **Intensity** and **Period**: Define the maximum number of restart activations allowed within a given period (in seconds). If this limit is exceeded, the supervisor terminates all child processes and itself, with the reason `act.ErrSupervisorRestartsExceeded`.
* **KeepOrder**: Specifies the order in which child processes are stopped when the restart strategy is activated. If this option is enabled, processes will be stopped sequentially in reverse order. By default, this option is disabled, meaning that all child processes are stopped simultaneously. Processes are restarted only after all child processes have fully stopped (either all processes for `act.SupervisorTypeAllForOne` or part of the processes for `act.SupervisorTypeRestForOne`).

### Methods of `act.Supervisor`

* `StartChild(name gen.Atom, args...)`: Starts a child process by its specification name. If arguments (`args`) are provided, they are updated in the specification for future restarts.
* `AddChild(child act.SupervisorChildSpec)`: Adds a child specification and automatically starts the process.
* `EnableChild(name gen.Atom)`: Enables and starts a previously disabled child process.
* `DisableChild(name gen.Atom)`: Disables the specification and stops the running process.
* `Children()`: Returns a list of `[]act.SupervisorChild` with data on specifications and running processes, sorted according to the child process specification list.

During initialization, the _supervisor_ defines the initial set of child processes, the _supervisor_ type, and the restart strategy for the child processes. `act.Supervisor` provides several methods for managing child processes dynamically and obtaining information about running processes:

\




