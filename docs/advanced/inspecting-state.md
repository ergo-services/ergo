---
description: HandleInspect as the observability surface of an actor, and what it makes possible
---

# Inspecting Actor State

An actor's state is private by design. Nothing outside it can read a field, and that is what makes the actor model safe: no locks, no shared memory, no races. It is also what makes a running actor system opaque. A debugger can pause one goroutine, but a node is thousands of them, and the interesting question is rarely about one process in isolation.

`HandleInspect` is the one sanctioned way out of that. It is not an API for other processes to use, not a message handler, and not a place to compute anything. It is the actor's answer to a single question, asked from the outside at an arbitrary moment: *what do you currently believe?*

```go
func (w *Worker) HandleInspect(from gen.PID, item ...string) map[string]string {
    return map[string]string{
        "state":       w.state,
        "queue_depth": fmt.Sprintf("%d", len(w.queue)),
        "last_error":  w.lastError,
    }
}
```

The mechanics are covered in [Actor](../actors/actor.md#inspection): requests arrive on the Urgent queue, values are strings, and the callback must return immediately. This page is about the part the mechanics do not tell you - what to put in there, and what it buys.

The division of labour is worth stating plainly, because it is unusual. The framework does not define what belongs in that map. It has no schema for it, no field registry, no notion of which keys are meaningful. All it does is instrument: deliver the request, call your callback, and carry the result to whoever asked - the observer renders it, [MCP](../extra-library/applications/mcp.md) returns it as a tool result, a sibling process gets it from `process.Inspect`. **The content is entirely yours, and so is the diagnostic value.** An actor whose callback returns three convenient fields is an actor that cannot be diagnosed, and no amount of tooling above it changes that.

## What you do not expose does not exist

This is the whole of it. Diagnosis is limited to the fields you chose to expose, and that choice is made months before the incident by someone who does not know what the incident will be.

The failure mode is specific and easy to walk into. An actor exposes the values that were easy to format - a name, a counter, a boolean - and omits the ones that carry the decision it just made. Everything looks reasonable in the inspection output, and the actual state is unreachable.

A concrete case. A leader-election actor exposed four fields: cluster id, term, a boolean "am I the leader", and a peer count. Every one of them is true and none of them is enough. The actor has three roles, not two - follower, candidate, leader - and a boolean cannot say which of the first two it is. So a replica stuck as a candidate for hours, campaigning and never winning, was indistinguishable from a healthy follower on every surface the system offered. The peer count said `3` without saying which three, so a stale entry pointing at a process that no longer existed looked identical to a live peer. Nothing reported whether an election timer was still armed, which is the difference between a replica that will try again and one that has stopped trying.

The state was recoverable, but only sideways: by comparing per-connection message counters between nodes to prove that a leader had emitted nothing for a day, and by taking four inspection readings twenty minutes apart to prove a term had stopped advancing. Each of those would have been one field.

The rule that follows is not "expose more". It is: **expose the state your code branches on.** If a line of your actor reads a field to decide what to do, an operator will eventually need to read the same field to understand what was done.

## Five fields that carry most of the value

In practice, five kinds of field carry almost all the diagnostic value.

**The derived role, not the raw flags.** If your actor has three states and you store two booleans, expose the resolved state as a word. Whoever reads it should not have to reconstruct your state machine from its parts.

**Identities, not counts.** `peers: 3` cannot be checked against reality. `peers: n1@host,n2@host,n3@host` can, and it is how a stale or duplicated entry becomes visible.

**Things you dropped silently.** Every place where your code decides to ignore a message is invisible by construction. A counter per reason turns a silent drop into evidence: `dropped: cluster_id_mismatch=2,stale_heartbeat=17`. This is usually the highest-value field in the map, because a message that was discarded leaves no other trace anywhere.

**Whether your timers are armed.** For any actor driven by `SendAfter`, "waiting to act" and "has stopped acting" look the same from outside. One boolean separates them. Note that testing the `gen.CancelFunc` you stored does not answer it: `SendAfter` fires once, and nothing clears your variable when it does, so a spent handle still looks armed. Track the arming explicitly or the field will lie.

**When the last transition happened.** A value plus the timestamp it last changed answers "is this stuck?" in one reading. Without the timestamp it takes two readings and a guess about how long to wait between them.

Keep it cheap. The callback runs on the actor's own goroutine, so while it runs the mailbox is not being drained. Format what you already hold; do not compute, do not call out, do not touch the network.

## Composing with an embedded behavior

`act.Actor`, `act.Pool`, `act.Router`, `act.Supervisor` and the extra-library actors all have state of their own worth reporting - pool statistics, restart counts, election state. If your implementation overrides `HandleInspect`, that state must not disappear.

The base behaviors handle this for you: they compute their own fields first and merge yours on top.

```go
case gen.MailboxMessageTypeInspect:
    items := message.Message.([]string)
    // own state first, the behavior may override any of the fields
    result := p.inspect(items...)
    for k, v := range p.behavior.HandleInspect(message.From, items...) {
        result[k] = v
    }
    p.SendResponse(message.From, message.Ref, result)
```

So a consumer adds fields, and may deliberately replace one, but cannot erase the rest. If you are implementing `HandleInspect` on top of one, you can rely on it: the framework's fields will be there beside yours.

The base behaviors namespace their own keys with a reserved `ergo:` prefix - `ergo:pool_size`, `ergo:children_total`, `ergo:state` and so on. That is what makes the merge safe in both directions: a field of yours cannot collide with one of theirs by accident, and you can still override one deliberately by naming it with the prefix. If you write a base behavior of your own, follow the same shape and the same prefix; if you are the consumer, keep your keys unprefixed and they will never clash.

## When the state is too large to return

Everything above assumes the answer fits in a map. Plenty of actors hold state that does not: a registry with a hundred thousand sessions, a scheduler with a deep queue, a cache with a million keys. Returning all of it is not an option - the callback must be cheap, and nobody can read it anyway.

The `item` arguments are the way out, and they are more than a field filter. Treat them as a small query vocabulary with three tiers.

**A bounded summary by default.** With no items, answer with aggregates only: totals, distribution, the extremes. The size of this answer must not depend on the size of the state.

**A `help` item that names what can be asked.** This is what makes the surface self-describing. A reader - human or agent - does not need to know your schema in advance; it asks once and learns the vocabulary. Nothing in the framework enforces this or knows about it: it is a convention you implement, which is precisely why it is worth implementing - without it the vocabulary exists only in your source.

**Parameterised items that drill into one entity.** `session <id>`, `user <id>`, `top slowest` - each returns detail about a small, named part of the state.

```go
func (m *SessionManager) HandleInspect(from gen.PID, item ...string) map[string]string {
    if len(item) == 0 {
        return m.summary() // totals only, never grows with len(m.sessions)
    }

    result := map[string]string{}
    for _, q := range item {
        switch {
        case q == "help":
            result["help"] = "summary keys: sessions_total, sessions_idle, oldest_age; " +
                "queries: session <id>, user <id>, top slowest [n], top oldest [n]"

        case strings.HasPrefix(q, "session "):
            id := strings.TrimPrefix(q, "session ")
            s, ok := m.sessions[id]
            if ok == false {
                result[q] = "<not found>"
                continue
            }
            result[q] = fmt.Sprintf("user=%s state=%s idle=%s bytes_in=%d",
                s.user, s.state, time.Since(s.lastSeen).Round(time.Second), s.bytesIn)

        case strings.HasPrefix(q, "user "):
            user := strings.TrimPrefix(q, "user ")
            ids := m.byUser[user] // pre-indexed, not a scan
            result[q] = fmt.Sprintf("sessions=%d %s", len(ids), joinCapped(ids, 20))

        case strings.HasPrefix(q, "top "):
            result[q] = m.top(strings.TrimPrefix(q, "top "))

        default:
            result[q] = "<unknown item>" // reported, not silently absent
        }
    }
    return result
}
```

Four things that matter in that shape.

**Cap every answer.** A query that can match a million entries must return the first `n` and say so. An unbounded answer reintroduces the problem the queries exist to avoid.

**Never scan.** `user <id>` above reads an index the actor already maintains. If answering a query means walking the whole state, either keep the index or do not offer the query - the callback holds the actor's own goroutine while it runs.

**Report an unknown item.** Returning nothing for a key the caller asked about is indistinguishable from a value that happens to be empty. `<unknown item>` costs one line and removes the ambiguity.

**Keep it read-only.** A query language over `item` is a good idea; a command language over it is not. Callers treat inspection as free - tools cache and retry it, an agent exploring a symptom calls it dozens of times - so anything that changes state belongs in `HandleCall` instead.

Answering queries also changes what the vocabulary itself tells a reader. `session <id>`, `user <id>`, `top slowest` says that this actor is indexed by session and by user and tracks latency, before anyone looks at a single value. That is diagnostic information in its own right: two actors with the same number of fields can differ enormously in how much they will let you ask, and `help` is where the difference becomes visible.

## From one process to the whole system

A single inspection call is a snapshot of one actor. What makes the callback worth designing carefully is that everything above it is built from those snapshots.

**One node, read by a human.** [Observer](observer.md) shows a process list, its tree, and the inspection output of whichever process you open, updating live. This is the view for "I know roughly where the problem is".

**A whole cluster, read by an AI.** [MCP](../extra-library/applications/mcp.md) exposes the same surface as tools an agent calls on demand: enumerate processes across the cluster, inspect any of them, follow the topology, sample profiles. Each node runs the diagnostic tools internally, and one entry point reaches all of them through cluster proxy, so a single conversation covers every node.

The difference between the two is not convenience, it is method. A dashboard answers questions decided in advance. An agent holding the whole inspection surface can work the other way round: start from a symptom, enumerate what exists, read the state of the processes that look implicated, correlate across nodes, and narrow down. Point diagnosis becomes system diagnosis, because nothing has to be selected up front.

**And it reads your source.** This is the part that changes the character of the work. An agent that can inspect live actor state and read the code that produced it is looking at cause and effect at once. The state says what the system believes; the source says which branch produced that belief and what it will do next. Neither alone is enough - a field value without the code is a number, and code without runtime state is a hypothesis - and together they close the loop that a debugger closes for a single-threaded program, but across a live distributed system that cannot be paused.

That is the reason to treat `HandleInspect` as a design surface rather than a debug convenience. The value of every layer above it - the observer view, the cluster-wide diagnostic, the agent that explains what it found - is bounded by whether the field it needed was exposed.

## Taken far enough, it is an admin view

Follow the query vocabulary to its conclusion and a summary plus a handful of drill-downs stops being a debug aid. Look up one entity by id, list what belongs to one tenant, show the worst ten by latency - that is the read half of an admin panel, over live state, for the cost of a switch statement. No separate service, no query layer, no HTTP handlers, no second deployment, and no copy of the data: the actor already holds the state, and it already maintains the indexes because it needs them to do its job. The observer and MCP supply the front end.

Worth being clear about the boundaries, so nobody builds the wrong thing on it.

**It is the read half only.** Commands go through `HandleCall`, for the reasons above.

**It is for operators, not end users.** There is no authentication on it beyond access to the node, no per-tenant scoping, and no audit trail. Whoever can reach the observer or the MCP entry point can ask anything the actor answers. That is the right trade for an engineering tool and the wrong one for a customer-facing feature.

**It is a diagnostic surface, not a data API.** Values are strings, keys are yours to rename, and nothing versions them. If another system needs this data, give it a proper request and response instead of parsing an inspection map.

## Checklist

Before shipping an actor, read your own `HandleInspect` against these:

- Does it report the role or phase as a word, rather than the flags it is derived from?
- Does it name the peers, children or targets it holds, rather than counting them?
- Does every branch that silently drops or ignores something have a counter here?
- If the actor is timer-driven, can a reader tell "waiting" from "stopped"?
- Does each value that can get stuck carry the time it last changed?
- If the state is large, is the default answer bounded, is there a `help` item, and is
  every query backed by an index rather than a scan?
- Does it return immediately, with no computation, no I/O, and no locks?
- If it embeds a base behavior, does the base state still come through?

## See also

- [Actor - Inspection](../actors/actor.md#inspection) - the callback's mechanics and constraints
- [Inspecting With Observer](observer.md) - the human-facing view of the same data
- [MCP](../extra-library/applications/mcp.md) - the cluster-wide, agent-facing view
- [AI Agents](../ai-agents.md) - using Ergo as both runtime and diagnostic surface
- [Debugging](debugging.md) - the wider set of techniques this fits into
