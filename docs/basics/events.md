---
description: Publish/Subscribe Event Mechanism
---

# Events

The actor model excels at point-to-point communication. Process A sends a message to process B. Process C makes a request to process D. Each interaction has a specific sender and receiver.

But some scenarios need one-to-many communication. A price feed updates and dozens of trading strategies need the new price. A user logs in and multiple subsystems need notification. A sensor reading arrives and various monitoring processes need to react. You could send individual messages to each interested process, but then the producer needs to track all consumers. When consumers come and go, the producer's consumer list becomes a maintenance burden.

Events solve this with publish/subscribe semantics. A producer registers an event and publishes values to it. Consumers subscribe to the event without the producer knowing who they are. The framework handles message distribution - when the producer publishes an event, all current subscribers receive it. Subscribers can come and go dynamically, and the producer's code doesn't change.

## Registering Events

A process becomes an event producer by calling `RegisterEvent` with an event name and options. The call returns a token - a unique reference that proves ownership. Only the process holding this token (or a process it delegates to) can publish events under this name.

```go
token, err := process.RegisterEvent("price_update", gen.EventOptions{
    Notify: true,
    Buffer: 10,
})
```

The `Notify` option controls whether the producer receives notifications about subscriber changes. When enabled, the producer receives `gen.MessageEventStart` when the first subscriber appears and `gen.MessageEventStop` when the last subscriber leaves. This allows the producer to start or stop expensive operations based on demand. If nobody's watching the price feed, why fetch prices? This option is ignored for events registered at the node level, since the node core does not consume such messages.

The `Buffer` option specifies how many recent events to keep. When a new subscriber joins, it receives the buffered events as a catch-up mechanism. Set this to zero if events are only relevant at the moment they're published. Set it to a reasonable number if new subscribers should see recent history.

Events are identified by name and node. The combination must be unique. Two processes on the same node can't register events with the same name. But processes on different nodes can register events with the same name - they're different events.

## Open Events

*Introduced in v3.3.0.*

By default, only the token holder can publish to an event. This prevents unauthorized processes from publishing events they don't own. For events that represent an internal node-wide bus, this protection is sometimes more friction than benefit. You end up distributing the token across multiple processes, or through environment variables, just so known participants can publish.

The `Open` option disables the token check on publish.

```go
token, _ := process.RegisterEvent("app.events", gen.EventOptions{
    Open:   true,
    Buffer: 50,
})
```

Any local process can now publish to this event by name, regardless of the token value. The owner check on `UnregisterEvent` is unaffected. Only the registering process (or the node, for node-level events) can unregister.

Consider a bus inside the node where application events land: "order created", "user signed up", "payment received". They come from different modules, and subscribers (notifier, analytics, search indexer) pick up whichever ones matter. Nobody owns the bus. Handing a shared token to every emitter is plumbing that protects nothing.

Open events trade the typo and bug protection that the token provides for simpler distribution. A process can accidentally publish to an event it was never supposed to touch. Use this option when the event is deliberately a shared bus and the token ceremony adds no real security in your context.

## Publishing Events

Publishing an event sends it to all current subscribers.

```go
process.SendEvent("price_update", token, PriceUpdate{Symbol: "BTC", Price: 42000})
```

You pass your application data directly. The framework wraps it in `gen.MessageEvent` automatically, adding the event identifier and timestamp. Subscribers receive the complete `gen.MessageEvent` structure containing your data.

The producer uses the token obtained during registration. If you try to publish with an incorrect token, the operation fails. This prevents unauthorized processes from publishing events they don't own.

Event publishing is fire-and-forget. The producer doesn't wait for acknowledgment or know how many subscribers received the event. The framework handles distribution asynchronously.

## Subscribing to Events

Processes subscribe to events through links or monitors, the same mechanisms used for process lifecycle tracking.

`LinkEvent` creates a link to an event. You receive event messages as they're published. If the event producer terminates or unregisters the event, you receive an exit signal. The link semantics apply - by default, you'd terminate too.

`MonitorEvent` creates a monitor on an event. You receive event messages and a down notification if the producer terminates or the event is unregistered, but you don't terminate automatically.

Both methods return buffered events upon successful subscription:

```go
lastEvents, err := process.LinkEvent(gen.Event{Name: "price_update", Node: "node@host"})
for _, event := range lastEvents {
    // Process historical events
    price := event.Message.(PriceUpdate)
}
```

The buffered events let subscribers catch up on what happened before they joined. If the buffer size was 10 and 5 events have been published, new subscribers receive those 5 events immediately.

For local events, you can omit the node name: `gen.Event{Name: "price_update"}`. The framework fills in the local node name. For remote events, specify the full event identifier including the remote node name.

## Event Lifecycle

Events exist from registration until unregistration or producer termination.

When you register an event, it becomes available for subscription. Processes on any node can subscribe if they know the event name and node. The framework tracks all subscribers and distributes published events to them.

When the producer terminates, the event is automatically unregistered. All subscribers receive termination notifications (exit signals for links, down messages for monitors). The event name becomes available for registration again.

The producer can explicitly unregister an event with `UnregisterEvent`. This triggers the same notifications to subscribers. Use this when you're done publishing events but your process continues running.

If a subscriber terminates or unsubscribes (via `UnlinkEvent` or `DemonitorEvent`), the producer doesn't receive notification unless `Notify` was enabled. With `Notify`, the producer receives `gen.MessageEventStop` when the last subscriber leaves.

## Node-Level Events

All examples so far registered events from a process. That process is the producer, and the event exists only as long as the process runs. When the process terminates, the event is unregistered and subscribers receive termination notifications. If another process later registers the same event name, subscribers must subscribe again.

Some events belong to the node itself, not to any particular process. Application events, health signals, internal buses. You want these events to exist for the entire lifetime of the node, regardless of which process currently publishes. The `gen.Node` interface provides `RegisterEvent` for this.

```go
token, err := node.RegisterEvent("notifications", gen.EventOptions{
    Open:   true,
    Buffer: 100,
})
```

The event's producer is the node core. It survives any publisher process coming and going. A process that restarts continues publishing to the same event after restart. Subscribers are not affected.

### Race on Subscription

*Introduced in v3.3.0.*

There is a timing problem with process-registered events. If a subscriber's `Init()` tries to `LinkEvent` before the producer process has called `RegisterEvent`, the link fails with `gen.ErrEventUnknown`. The subscriber then needs retry logic or some other coordination mechanism.

The `NodeOptions.Events` field registers node-level events before any application is started.

```go
options := gen.NodeOptions{
    Events: []gen.NodeEventSpec{
        {Name: "notifications", Buffer: 100},
        {Name: "audit", Buffer: 10},
    },
    Applications: []gen.ApplicationBehavior{...},
}

node, err := ergo.StartNode("mynode@localhost", options)
```

Events declared here are registered as open events with the node as producer. By the time the first application starts, these events already exist. Any process can subscribe from `Init()` without a race. Any process can publish by name.

If your event requires the token check and you only want a specific process to publish, register it imperatively via `node.RegisterEvent(..., gen.EventOptions{Open: false})` and distribute the token through environment variables or process arguments.

## Network Transparency

Events work across nodes seamlessly. A producer on node A can publish events that subscribers on nodes B, C, and D receive. The framework handles the network distribution.

When you subscribe to a remote event, the framework sends a subscribe request to the remote node. The remote node records your subscription. When the producer publishes an event on the remote node, the remote node sends it to all remote subscribers, including you.

If the network connection fails, subscribers receive termination notifications with reason `gen.ErrNoConnection`. This is consistent with how links and monitors handle network failures for processes.

The buffered events work across nodes too. When you subscribe to a remote event, the remote node sends you the buffered events as part of the subscription response. This catch-up mechanism works regardless of where the producer and subscribers are located.

## Token Delegation

Event tokens can be delegated. The producer can give its token to another process, allowing that process to publish events under the producer's event registration.

This enables patterns where event generation is separated from event registration. A coordinator registers the event and distributes the token to worker processes. Workers publish events as data becomes available. Subscribers don't know or care which process instance published each event - they just receive events on the registered event name.

Token delegation also allows rotating producers. A primary process registers an event and holds the token. A backup process can take over using the same token if the primary fails. Subscribers see a continuous event stream even as the producing process changes.

## Event Messages

Event messages have a specific structure:

Each `gen.MessageEvent` contains:
- **Event** - The event identifier (name and node)
- **Message** - Your application data (any type)
- **Timestamp** - When the event was published (nanoseconds since epoch)

Subscribers receive these wrapped messages and extract the application data. The wrapping provides context: which event this came from, when it was published, allowing subscribers to handle events from multiple sources or correlate timing.

## Event Statistics

Each registered event tracks per-event counters: how many messages were published, how many were delivered to local subscribers, and how many were sent to remote nodes. These counters are available through `Node.EventInfo` and `Node.EventRangeInfo`.

To query a specific event:

```go
info, err := node.EventInfo(gen.Event{Name: "price_update", Node: "node@host"})
// info.MessagesPublished  - total messages published to this event
// info.MessagesLocalSent  - messages delivered to local subscribers
// info.MessagesRemoteSent - messages sent to remote subscriber nodes
// info.Subscribers        - current subscriber count
```

To iterate over all registered events on the node:

```go
node.EventRangeInfo(func(info gen.EventInfo) bool {
    fmt.Printf("event %s: published %d, local %d, remote %d\n",
        info.Event.Name,
        info.MessagesPublished,
        info.MessagesLocalSent,
        info.MessagesRemoteSent,
    )
    return true // continue iteration
})
```

Node-level aggregate counters are also available in `gen.NodeInfo` via `node.Info()`: `EventsPublished` (local producer publishes), `EventsReceived` (events arriving from remote nodes), `EventsLocalSent`, and `EventsRemoteSent`.

The [Metrics actor](../extra-library/actors/metrics.md) automatically exports these counters as Prometheus metrics, along with per-event top-N breakdowns by subscribers, published, local deliveries, and remote sent. It also tracks event utilization state: whether events are actively used, waiting on demand, or idle.

## Practical Patterns

Events fit several common scenarios.

**Data streaming** - A sensor process registers an event and publishes readings. Multiple monitoring processes subscribe. Each reading goes to all monitors. If a monitor crashes and restarts, it subscribes again and receives recent buffered readings to catch up.

**State change notification** - A user session process registers an event and publishes state changes (login, logout, permission change). Authorization processes subscribe and update their caches. The session process doesn't track who's interested in its state changes.

**System telemetry** - Processes publish metrics as events. Monitoring processes subscribe and aggregate. If the monitoring process restarts, buffered events provide recent history to rebuild state.

**Workflow coordination** - An order processing system publishes order state events. Inventory, shipping, and billing processes subscribe. Each subsystem reacts to relevant state changes. The order process doesn't orchestrate the subsystems - they coordinate through events.

For more information on links and monitors as they apply to processes and nodes, see the [Links and Monitors](links-and-monitors.md) chapter.
