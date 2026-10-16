---
description: HandleInspect as the observability surface of an actor, and what it makes possible
---

# Inspecting Actor State

The runtime can report a process's mailbox and lifecycle, but only the actor knows which job it is waiting on or which peers it believes are present. `HandleInspect` exposes that state as a map of strings while the actor is running.

An inspection request is handled on the actor's dispatcher. It cannot interrupt a callback that is stuck, and it must itself return quickly. This chapter describes useful fields and bounded queries that Observer and the MCP endpoint can read.

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

The division of labour is worth stating plainly, because it is unusual. The framework does not define what belongs in that map. It has no schema for it, no field registry, no notion of which keys are meaningful. All it does is instrument: deliver the request, call your callback, and carry the result to whoever asked - the observer renders it, its [MCP surface](../extra-library/applications/observer.md) returns it as a tool result, a sibling process gets it from `process.Inspect`. **The content is entirely yours, and so is the diagnostic value.** An actor whose callback returns three convenient fields is an actor that cannot be diagnosed, and no amount of tooling above it changes that.

## What you do not expose does not exist

An operator can read only the fields the actor exposes. Choose fields that explain its decisions, including paths that wait, reject work or discard a message. A counter alone may show activity without explaining why progress stopped.

The failure mode is specific and easy to walk into. An actor exposes the values that were easy to format - a name, a counter, a boolean - and omits the ones that carry the decision it just made. Everything looks reasonable in the inspection output, and the actual state is unreachable.

Consider a leader-election actor that exposes a cluster ID, term, leader boolean and peer count. A candidate repeatedly losing elections and a healthy follower both report `leader=false`. The current implementation also has an `unclustered` role. Report the resolved role so these conditions can be distinguished. Likewise, `peers=3` says nothing about which three peers are present or reachable. Timer state helps distinguish a candidate waiting to retry from one that has stopped scheduling elections.

Without those fields, diagnosis requires indirect evidence. Comparing connection counters can show that heartbeats stopped; repeated term readings can show that elections stopped advancing. Explicit timestamps and timer state let an operator answer these questions from the actor itself.

Start with the state used in decision branches. If a handler chooses an action based on a field, that field or its derived meaning is a useful candidate for inspection. Keep the default output bounded, as the later examples demonstrate.

## Five fields that carry most of the value

The following five kinds of field are a useful starting point. Which ones matter depends on the actor's state machine.

**The derived role, not the raw flags.** If your actor has three states and you store two booleans, expose the resolved state as a word. Whoever reads it should not have to reconstruct your state machine from its parts.

**Identities, not counts.** `peers: 3` cannot be checked against reality. `peers: n1@host,n2@host,n3@host` can, and it is how a stale or duplicated entry becomes visible.

**Things you dropped silently.** Every place where your code decides to ignore a message is invisible by construction. A counter per reason turns a silent drop into evidence: `dropped: cluster_id_mismatch=2,stale_heartbeat=17`. This is usually the highest-value field in the map, because a message that was discarded leaves no other trace anywhere.

**Whether your timers are armed.** For any actor driven by `SendAfter`, "waiting to act" and "has stopped acting" look the same from outside. One boolean separates them. Note that testing the `gen.CancelFunc` you stored does not answer it: `SendAfter` fires once, and nothing clears your variable when it does, so a spent handle still looks armed. Track the arming explicitly or the field will lie.

**When the last transition happened.** A value plus the timestamp it last changed answers "is this stuck?" in one reading. Without the timestamp it takes two readings and a guess about how long to wait between them.

`HandleInspect` runs on the actor dispatcher. Format existing state and return promptly; network calls, long computations and waits delay the actor's other work.

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

Your returned fields are merged over the base fields. Naming an existing key deliberately replaces its value.

Base keys use the reserved `ergo:` prefix, such as `ergo:pool_size`. Keep application keys unprefixed to avoid collisions. Use the prefix only when deliberately overriding a framework field.

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

Queries need explicit bounds:

**Cap every answer.** A query that can match a million entries must return the first `n` and say so. An unbounded answer reintroduces the problem the queries exist to avoid.

**Never scan.** `user <id>` above reads an index the actor already maintains. If answering a query means walking the whole state, either keep the index or do not offer the query - the callback holds the actor's own goroutine while it runs.

**Report an unknown item.** Returning nothing for a key the caller asked about is indistinguishable from a value that happens to be empty. `<unknown item>` costs one line and removes the ambiguity.

**Keep it read-only.** A query language over `item` is a good idea; a command language over it is not. Callers treat inspection as free - tools cache and retry it, an agent exploring a symptom calls it dozens of times - so anything that changes state belongs in `HandleCall` instead.

A `help` response should name the available query forms and their limits. It lets an operator or client discover whether the actor can answer by session, user or another index without reading its source first.

## From one process to the whole system

Observer and the MCP endpoint build their application-state views from individual inspection responses.

**One node, read by a human.** [Observer](observer.md) shows a process list, its tree, and the inspection output of whichever process you open, updating live. This is the view for "I know roughly where the problem is".

**A whole cluster, read by an AI.** The [MCP surface of Observer](../extra-library/applications/observer.md) exposes the same inspection as resources and tools an agent asks for on demand: enumerate processes across the cluster, inspect any of them, follow the topology, capture a profile. Every node runs the built-in `system` application that answers those questions, and the one node serving MCP reaches all of them, so a single conversation covers the cluster.

The web UI presents selected views. An MCP client can request those observations in an order chosen during an investigation and correlate results across nodes.

**Correlating with source.** A client with repository access can compare reported state with the branch that produced it. That helps interpret fields, but the observations are snapshots taken at different times; they do not pause the system.

Expose the state needed to explain decisions, including rejected inputs and pending transitions. Generic runtime counters cannot supply it.

## Taken far enough, it is an admin view

Bounded queries can also support an operator view: one entity by ID, the entries for a tenant or the slowest ten operations. The actor answers from its current state and indexes. Observer and MCP can display those answers, subject to the access controls described below.

Keep these boundaries explicit:

**It is the read half only.** Commands go through `HandleCall`, for the reasons above.

**Operator access.** Inspection has no built-in per-tenant filtering or audit log. Observer may authenticate callers and restrict access with ceilings, but an allowed inspection sees whatever the actor returns. Do not expose customer data through it without appropriate scoping.

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
- [Observer](../extra-library/applications/observer.md) - the same data as an agent-facing MCP surface
- [AI Agents](../ai-agents.md) - using Ergo as both runtime and diagnostic surface
- [Debugging](debugging.md) - the wider set of techniques this fits into
