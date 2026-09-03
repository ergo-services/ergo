---
description: Vet Tool for Ergo Actor Model Invariants
---

# Actor Model Vet Tool

The actor model rests on rules the Go compiler cannot check. A message must not share memory with its sender. A callback must not block without a bound. A goroutine started inside a callback must not reach back into actor state. Break any of these and the code compiles, the tests pass, and the failure arrives later - as a race under load, as a mailbox that never drains, as a node that reports itself healthy while serving nothing.

`argus` reads your packages and reports those breaks. It is a vet tool, so it runs the way `go vet` runs: over the build graph, with your build tags, cached per package.

## Installation

```
go install ergo.tools/argus@latest
```

## Running it

In production, as a vet tool. The go command drives it and caches the results:

```
go vet -vettool=$(which argus) ./...
```

During development, directly over package patterns:

```
argus ./...
```

Package patterns resolve against the main module of the working directory, exactly as the go command resolves them. To analyse a module you are not standing in, move there first:

```
argus -C ../../application/observer ./...
```

## Reading a finding

```
gen/cron_action.go:103:4: [tier2] [A2011] []any is passed to Spawn and shares
unsynchronized memory with the parent: []interface{} -> any; the child runs on its
own goroutine, so pass a copy or a type that guards itself
```

Every finding carries a tier and a rule ID. The tier says how much to trust it, and decides whether it fails your build:

| Tier | Default | Meaning |
|------|---------|---------|
| 1 | error | The invariant is broken. The failure is a matter of timing, not of whether. |
| 2 | warning | The construct is wrong often enough to look at, and legitimate sometimes. |
| 3 | off | Style and hygiene. Opt in when you want it. |

The rule ID explains itself:

```
argus help          every rule in this build
argus help A1002    one rule in full - what it reports, what it deliberately does not
```

Read that second command before arguing with a finding. Each rule documents its own blind spots, and several of them are narrower than their titles suggest.

## Rules

Rules are grouped by what they protect.

**A1xxx - the actor model itself.** Shared memory in a message (A1001), an unbounded wait in a callback (A1002), a call to your own identity (A1003), a goroutine touching actor state (A1004), meta state written from both meta goroutines (A1005), sending a field of your own state (A1006), internal memory escaping across an actor boundary (A1007), `HandleCall` returning an error in the reason slot (A1008), a goroutine with no panic boundary (A1010), a blocking request in `Init` against the init budget (A1011), routing through the node handle from inside an actor (A1012).

**A2xxx - using the framework as it works.** A state-gated API called where its state forbids it (A2001), a discarded result hiding a certain failure (A2001a), a `HandleCall` that can never reply (A2002) or replies twice (A2003), a message type that cannot be serialized (A2004), a supervisor spec the framework rejects at init (A2005), timer misuse (A2006), an event that can never be published (A2007), a web request never completed (A2008), a type registration that fails at startup (A2009), a spawn argument shared with the parent (A2011), logging a transient failure and then returning it (A2012), discarding the buffered events a subscription returns (A2013), a round trip in `Terminate` (A2014), `Terminate` dereferencing what `Init` may never have assigned (A2014a), a self-timer chain armed from two places (A2015), identity established in a handler `Init` sent to itself (A2016), `Init` returning a framework sentinel as control flow (A2017), `Notify` set while the producer handles neither start nor stop (A2018).

**A3xxx - conventions.** A message type with no marker (A3001), a message type missing from the registration list (A3003), suppression debt (A3005), prose that should be a marker (A3006), a compression threshold below the framework floor (A3007).

## Adopting it on existing code

The first run on a codebase that has never seen the tool reports findings that are all true and none of which are getting fixed today. Take a baseline, and the run goes quiet until something new appears:

```
argus -argusmodel.keys ./... 2>&1 | argus baseline > argus-baseline.json
```

Findings already in the baseline stay silent. Anything new is reported.

## What to expect on real code

A codebase written before the tool existed reports plenty. `application/observer` reports A1001 broadly, because any message carrying a `map[string]any` or a slice shares memory on local delivery, and local delivery does not copy. Those findings are correct; whether they matter depends on whether the sender touches the value afterwards.

That is the tool's shape in general: it reports what it can prove about the construct, and leaves the judgement about your specific case to you. Read new findings as signal, not as verdicts.
