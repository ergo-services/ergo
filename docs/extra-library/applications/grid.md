# Grid

In a cluster, processes on different nodes need to find each other by name, know when one appears or disappears, and broadcast to a set of interested processes that changes over time. Building this by hand means maintaining monitors between nodes, tracking which node hosts what, and agreeing on a naming scheme, all while nodes join and leave.

Grid provides three capabilities as an application: a distributed registry that maps a key to a single owner process, lifecycle monitors that notify you when keys appear, change, or vanish, and process groups for per-key publish/subscribe.

Grid runs as an application on your node. Every node keeps a full local copy of the registry, so a lookup is a local read that returns immediately. Writes are serialized per key by a shard actor and replicate to peer nodes in the background. Grid is AP and eventually consistent: it stays available during a partition and converges when the partition heals, at the cost of brief windows where two nodes may disagree.

## Adding to Your Node

```go
import (
    "ergo.services/application/grid"
    "ergo.services/ergo"
    "ergo.services/ergo/gen"
)

func main() {
    node, err := ergo.StartNode("mynode@localhost", gen.NodeOptions{
        Applications: []gen.ApplicationBehavior{
            grid.CreateApp(grid.Options{Domain: "grid", Shards: 8}),
        },
    })
    if err != nil {
        panic(err)
    }
    node.Wait()
}
```

Every node that starts Grid with the same `Domain` discovers the others and forms a mesh - through the registrar, already-connected nodes, or static `Peers`. Once the node is up, any actor on it uses the registry, monitors, and groups through the `grid` package. No wiring between nodes is required.

## Configuration

```go
grid.Options{
    Domain:    "grid",              // peering scope; application name is grid_<Domain>
    Shards:    8,                   // keyspace shard count; all nodes must agree
    Separator: "/",                 // key hierarchy separator for MonitorPrefix
    Peers:     []gen.Atom{"a@host"} // static seeds for discovery without a registrar
}
```

| Option | Default | Description |
|--------|---------|-------------|
| Domain | `"default"` | Peering scope. A node peers only with grids of the same domain. The Ergo application name is `grid_<Domain>`, so several independent grids can coexist on one node. |
| Shards | `8` | Number of shard actors the keyspace is split across. All nodes in a domain must use the same value - a peer with a different shard count is rejected during the handshake. |
| Separator | `"/"` | The hierarchy separator used by `MonitorPrefix`. A prefix matches the key itself and everything below it at a separator boundary. |
| Peers | none | Static seed nodes to contact for discovery when no registrar is available. Grid keeps trying to reach them. |

## How It Works

Each node keeps a full local copy of the registry. A key is hashed into one of `Shards` buckets, and one shard actor per bucket is the sole writer for that slice of the keyspace on the node:

```
Register("order/42") on node A

  fnv32a("order/42") % Shards = 3
        │
        ▼
  shard_3 @ A  ──replicate──▶  shard_3 @ B
 (sole writer)                 shard_3 @ C
```

Because a single actor owns each slice, writes to a key are serialized without locks. When a shard applies a local write it replicates it to the counterpart shard - the same index - on every peer node. Replication is asynchronous and fire-and-forget: `Register` returns as soon as the local shard has recorded the entry, and peers converge shortly after.

Reads never touch a shard actor. `Lookup` and the counts read the node's local copy directly, so they are cheap and lock-free. The consequence is the AP contract: **what you observe** is the node's converged view, which may briefly lag a write made on another node; **what happens** underneath is background replication that brings every node to the same state. If you need a read to reflect the very latest cluster-wide write with no lag, grid is not the tool - it trades that guarantee for availability and speed.

## The Registry

`Register` claims a key for the calling process. The owner is the caller's PID, and the `meta` value is arbitrary data carried alongside the entry.

```go
func (w *worker) Init(args ...any) error {
    if err := grid.Register(w, "grid", "order/42", "meta-v1"); err != nil {
        return err
    }
    return nil
}
```

`Register` is synchronous - it routes to the owning shard and returns an error. If a different, live process already owns the key it returns `gen.ErrTaken`. Registering the same key again as its current owner is idempotent when the metadata is unchanged; when the metadata differs, the entry is updated and monitors are notified. `Unregister` removes a key, but only if the caller owns it locally - otherwise it returns `gen.ErrUnknown` (no such key) or `gen.ErrIncorrect` (not the owner). When the owner process terminates, its keys are removed automatically.

`Lookup` reads the local view and returns the owner, the metadata, and whether the key exists:

```go
if pid, meta, ok := grid.Lookup(w, "grid", "order/42"); ok {
    w.Log().Info("order/42 owned by %s (%v)", pid, meta)
}
```

`RegistryCount` returns the size of the local view, which converges to the cluster-wide total. `LocalRegistryCount` and `LocalEntries` return only the keys owned by the calling node, which is what you want for handoff, draining, or inspection.

## Conflict Resolution

Because grid is AP, two nodes can register the same key during a partition, or at nearly the same instant. When their writes meet, grid resolves the conflict deterministically, last-writer-wins:

```
winner = later Time  →  higher PID.ID  →  higher PID.Creation  →  greater Node
```

The registration timestamp decides; ties break by owner PID, then by node name, so every node picks the same winner without coordination. The losing owner is stopped with `ErrRegistryConflict`, delivered as an exit signal, and the winner's entry is re-replicated to heal any peer that still holds the loser. After convergence the key has exactly one owner cluster-wide.

This is a deliberate trade-off. Last-writer-wins keeps the registry available and self-healing with no consensus round, but it means a registration you thought succeeded can later be revoked, and the process that lost is terminated. Grid's registry is a fast observation and coordination layer, not a distributed lock. If your application requires at-most-one ownership with no window of divergence - for example, exclusive control of an external resource - use grid to observe and route, and pair it with a linearizable authority for the actual exclusivity.

## Monitoring Keys

Monitors tell an actor when keys change. You subscribe to an exact key, a prefix, or the whole domain, and notifications arrive in `HandleMessage`.

```go
func (o *observer) Init(args ...any) error {
    return grid.MonitorPrefix(o, "grid", "order")
}

func (o *observer) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case grid.MessageRegistered:
        o.Log().Info("%s registered at %s", m.Key, m.Owner)
    case grid.MessageUpdated:
        o.Log().Info("%s meta changed to %v", m.Key, m.Meta)
    case grid.MessageUnregistered:
        o.Log().Info("%s gone: %s", m.Key, m.Reason)
    }
    return nil
}
```

On subscribe you first receive a `MessageRegistered` for every matching key already present, then live changes as they happen. This snapshot-then-stream behaviour lets an actor build its view in one place: whatever exists now arrives as if it had just been registered. Subscriptions are keyed by scope, so re-subscribing the same scope is a no-op, and they survive a shard restart.

The three subscription functions differ in scope and reach:

| Function | Scope | Shards contacted |
|----------|-------|------------------|
| `MonitorKey` | one exact key | the key's owning shard |
| `MonitorPrefix` | a key and everything below it | all shards |
| `MonitorAll` | every key in the domain | all shards |

`MonitorPrefix` matches at `Separator` boundaries rather than by raw bytes. With the default separator `/`, `MonitorPrefix("order")` matches `order` and `order/42` but not `order42` or `orders/1`. You do not need a trailing separator; if you supply one, it is honoured as given.

`MessageUnregistered` carries a `Reason` so a consumer can tell an orderly removal from a failure:

| Reason | Fires when |
|--------|------------|
| `ReasonUnregister` | the owner called `Unregister` |
| `ReasonDown` | the owner process terminated |
| `ReasonConflict` | the owner lost a last-writer-wins conflict |
| `ReasonNodeDown` | the owner's node left the cluster |

Cancel a subscription with `DemonitorKey`, `DemonitorPrefix`, or `DemonitorAll`, passing the same scope you monitored. When a subscribing process terminates, its subscriptions are dropped automatically.

## Process Groups

A group is per-key publish/subscribe layered on the Ergo event bus. The process that owns the key opens a group; other processes join it to receive broadcasts. The owner broadcasts with `Dispatch`, and payloads arrive at members in `HandleEvent`.

```go
// owner: opens a group for the key it owns and broadcasts to it
func (w *worker) Init(args ...any) error {
    grid.Register(w, "grid", "room/42", nil)
    return grid.OpenGroup(w, "grid", "room/42")
}

func (w *worker) HandleMessage(from gen.PID, message any) error {
    return grid.Dispatch(w, "grid", "room/42", message)
}

// member: joins when it sees the key, receives dispatches in HandleEvent
func (m *member) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case grid.MessageRegistered:
        m.joined, _ = grid.Join(m, msg.Domain, msg.Key)
    }
    return nil
}

func (m *member) HandleEvent(message gen.MessageEvent) error {
    switch message.Message.(type) {
    case gen.MessageDownEvent:
        // the group's owner went away; re-Join once the key reappears
    default:
        m.Log().Info("broadcast: %v", message.Message)
    }
    return nil
}
```

`Join` resolves the owner's node from the local registry, subscribes to the group event there, and returns the event handle to pass to `Leave`. Because a group is hosted by its owner, `Dispatch` and `MemberCount` must run on the owner node. A group lives exactly as long as its owner: when the owner terminates or its node leaves, members receive a `gen.MessageDownEvent`. The recovery pattern is to re-`Join` once the key reappears in the registry - the object has moved, and members follow it.

Groups report a member count but do not enumerate members, and membership is not replicated as registry state. When you need a roster, per-member join and leave events, or membership that outlives the owner, build that on grid's registry and monitors rather than on the group event.

## Peer Discovery

Each shard discovers and connects to its counterpart on other nodes on its own. It draws candidate nodes from three sources: the static `Peers` list, the nodes already connected to this one, and the registrar's answer for the application `grid_<Domain>`. It then handshakes with the counterpart shard of the same index. A peer is accepted only if it agrees on the domain, the shard index, and the shard count; a mismatch is logged and refused.

Static seeds are retried until they answer - quickly at first, then at a slower steady interval - so a seed that starts later still joins the mesh. When a peer's node disconnects or its grid application stops, the shard purges that node's keys from its local view and notifies monitors with `ReasonNodeDown`. When a peer connects, the two shards exchange an authoritative snapshot of the keys each owns; the snapshot reconciles deletes as well as additions, without clobbering entries that are newer than it, so a rejoining node cannot resurrect a key that was already removed.

## EDF Registration

Registry metadata and group payloads cross the network. A non-primitive `meta` value passed to `Register`, and any custom type passed to `Dispatch`, must be registered for the framework's encoding (EDF) by the consumer application, exactly as for any other message that travels between nodes. Primitive values need no registration.

## Relationship to Ergo Primitives

Grid is a convenience layer over primitives the framework already provides: process registration, `MonitorPID` and event links, and `gen.Event` publish/subscribe. It bundles them into a cluster-wide, sharded, self-healing package so you do not assemble discovery, cross-node monitoring, and broadcast by hand for the common case.

Reach past grid to those primitives when your requirements fall outside its trade-offs: a linearizable authority when you need strict single ownership rather than eventual convergence, `gen.Event` directly when a group needs an enumerable roster or per-member events, or `MonitorPID` directly for a single well-known process where a distributed registry is more than you need.
