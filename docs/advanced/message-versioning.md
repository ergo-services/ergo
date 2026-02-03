---
description: Evolving message contracts in distributed clusters
---

# Message Versioning

Distributed systems evolve. Services gain features, data models change, and nodes run different code versions during rolling upgrades. This article explains how to organize message versioning in Ergo Framework clusters.

## Why Explicit Versioning

EDF (Ergo Data Format) serializes messages by their Go type. Unlike Protobuf or Avro, EDF does not provide automatic backward compatibility. Changing struct fields creates an incompatible type.

This is intentional. Implicit compatibility hides breaking changes until runtime. Explicit versioning surfaces incompatibilities at compile time.

The approach:

```go
// Version 1
type OrderCreatedV1 struct {
    OrderID int64
}

// Version 2 - new field
type OrderCreatedV2 struct {
    OrderID  int64
    Priority int
}
```

Both types coexist. Sender chooses which to send. Receiver handles both:

```go
func (a *Actor) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case OrderCreatedV1:
        return a.handleOrderV1(m)
    case OrderCreatedV2:
        return a.handleOrderV2(m)
    }
    return nil
}
```

Register all versions at startup:

```go
func init() {
    types := []any{
        OrderCreatedV1{},
        OrderCreatedV2{},
    }
    for _, t := range types {
        if err := edf.RegisterTypeOf(t); err != nil && err != gen.ErrTaken {
            panic(err)
        }
    }
}
```

For details on EDF and type registration, see [Network Transparency](../networking/network-transparency.md).

## Message Scopes

Messages fall into two categories based on their nature.

### Private Messages

Direct communication between specific services. Request/response patterns between known parties.

```mermaid
flowchart LR
    Order[Order Service] -- ChargeRequest --> Payment[Payment Service]
    Payment -- ChargeResponse --> Order
```

**Owner:** receiver

Payment Service defines what it accepts. Order Service adapts to Payment's contract.

### Cluster-Wide Events

Domain events published to multiple subscribers. Any service can subscribe.

```mermaid
flowchart LR
    Order[Order Service] -- OrderCreatedV1 --> Analytics
    Order -- OrderCreatedV1 --> Notify
    Order -- OrderCreatedV1 --> Audit
```

**Owner:** shared repository

Events represent domain facts, not service-specific contracts. Ownership belongs to a shared module that all services import.

For event publishing patterns, see [Events](../basics/events.md).

## Repository Organization

Separate private contracts from cluster-wide events:

```
company.com/
│
├── events/                     # cluster-wide events
│   ├── go.mod                  # module company.com/events
│   ├── order_created_v1.go
│   ├── order_created_v2.go
│   ├── payment_received_v1.go
│   └── register.go
│
├── payment-api/                # Payment Service contract
│   ├── go.mod                  # module company.com/payment-api
│   ├── charge_v1.go
│   └── refund_v1.go
│
├── order-service/
│   ├── go.mod                  # requires: events, payment-api
│   ├── internal/
│   └── cmd/
│
└── payment-service/
    ├── go.mod                  # requires: events
    ├── internal/
    └── cmd/
```

### Module Versioning

Keep module path at v1. Version in type names:

```go
// events/order_created_v1.go
package events

type OrderCreatedV1 struct {
    EventID   string
    OrderID   int64
    CreatedAt int64
}
```

```go
// events/order_created_v2.go
package events

type OrderCreatedV2 struct {
    EventID   string
    OrderID   int64
    Priority  int
    CreatedAt int64
}
```

This avoids Go modules v2+ path changes and keeps imports stable across the cluster.

### Registration Helper

```go
// events/register.go
package events

import (
    "ergo.services/ergo/gen"
    "ergo.services/ergo/net/edf"
)

func init() {
    types := []any{
        OrderCreatedV1{},
        OrderCreatedV2{},
        PaymentReceivedV1{},
    }
    for _, t := range types {
        if err := edf.RegisterTypeOf(t); err != nil && err != gen.ErrTaken {
            panic(err)
        }
    }
}
```

Importing the `events` package triggers `init()` and registers types automatically.

For message isolation patterns within a single codebase, see [Project Structure](../basics/project-structure.md).

## Ownership Rules

| Scope | Owner | Module | Changes approved by |
|-------|-------|--------|---------------------|
| Private messages | Receiver | `receiver-api/` | Receiver team |
| Cluster-wide events | Shared | `events/` | All consumers |

The receiver owns private contracts because it implements the logic. Multiple senders may use the same contract, but they all adapt to what the receiver accepts. This follows the [Consumer-Driven Contracts](https://martinfowler.com/articles/consumerDrivenContracts.html) pattern. Events are shared because they represent domain facts, not service-specific APIs.

### Private Contract Ownership

Payment Service owns its API contract:

```go
// payment-api/charge_v1.go
package paymentapi

type ChargeRequestV1 struct {
    OrderID int64
    Amount  int64
}

type ChargeResponseV1 struct {
    TransactionID string
    Status        string
}
```

Order Service imports and uses it:

```go
import paymentapi "company.com/payment-api"

response, err := a.Call(paymentPID, paymentapi.ChargeRequestV1{
    OrderID: order.ID,
    Amount:  order.Total,
})
```

Payment team decides when to create V2. Order team adapts.

### Cluster Event Ownership

Events require broader coordination:

```
events/
├── OWNERS.md           # who approves changes
├── CHANGELOG.md        # version history
└── order/
    ├── created_v1.go
    └── created_v2.go
```

```markdown
# OWNERS.md

Maintainers (approve all changes):
- platform-team

Reviewers (approve breaking changes):
- order-team
- payment-team
- analytics-team
```

Breaking changes require sign-off from all consumers.

## Version Lifecycle

### When to Create New Version

Create V2 when:
- Adding field
- Removing field
- Changing field type
- Renaming field
- Reordering fields
- Changing field semantics

In EDF, any struct modification requires a new version. Even changing field order creates an incompatible type. There are no "optional fields" like in Protobuf.

### Deprecation

Mark deprecated versions:

```go
// Deprecated: use ChargeRequestV2. Remove after 2025-Q3.
type ChargeRequestV1 struct {
    OrderID int64
    Amount  int64
}
```

Log when receiving deprecated versions:

```go
case ChargeRequestV1:
    a.Log().Warning("deprecated ChargeRequestV1 from %s", from)
    return a.handleChargeV1(m)
```

### Removal

Remove only when:
1. All senders upgraded to V2
2. Monitoring confirms zero V1 traffic
3. Deprecation period passed

Remove in order:
1. Stop accepting (return error for V1)
2. Remove from registration
3. Delete type definition

## Compatibility Rules

EDF enforces strict type identity. Any struct change breaks wire compatibility.

| Change | Compatible | Action |
|--------|------------|--------|
| Add field | No | Create V2 |
| Remove field | No | Create V2 |
| Change field type | No | Create V2 |
| Rename field | No | Create V2 |
| Reorder fields | No | Create V2 |

This differs from Protobuf/Avro where adding optional fields is compatible. In EDF, every change requires explicit versioning.

## Rolling Upgrades

During rolling upgrades, nodes run different code versions simultaneously.

### Upgrade Strategy

1. **Deploy V2 types** to shared module
2. **Update receivers** to handle V1 and V2
3. **Rolling restart** receiver nodes
4. **Update senders** to send V2
5. **Rolling restart** sender nodes
6. **Deprecate** V1 after all nodes upgraded
7. **Remove** V1 after deprecation period

### Coexistence Period

```mermaid
flowchart LR
    subgraph "During Rolling Upgrade"
        S1[Sender\nold code] -- V1 --> R1[Receiver\nnew code\nhandles V1 + V2]
        S2[Sender\nnew code] -- V2 --> R2[Receiver\nnew code\nhandles V1 + V2]
    end
```

Receivers must support both versions during the upgrade window.

For deployment patterns with weighted routing, see [Building a Cluster](building-a-cluster.md).

## Anti-Corruption Layer

When handling multiple versions, isolate translation logic:

```go
// internal/acl/charge.go
package acl

import api "company.com/payment-api"

func ChargeV1ToV2(v1 api.ChargeRequestV1) api.ChargeRequestV2 {
    return api.ChargeRequestV2{
        OrderID:  v1.OrderID,
        Amount:   v1.Amount,
        Currency: "USD", // default for V1 clients
    }
}
```

Use in handler:

```go
func (a *Actor) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case api.ChargeRequestV1:
        v2 := acl.ChargeV1ToV2(m)
        return a.processCharge(v2)
    case api.ChargeRequestV2:
        return a.processCharge(m)
    }
    return nil
}
```

Single implementation handles V2. ACL converts V1 to V2. When V1 is removed, delete the ACL function.

## Idempotency

Include unique identifier for deduplication:

```go
type OrderCreatedV1 struct {
    EventID   string    // UUID, unique per event instance
    OrderID   int64
    CreatedAt int64
}
```

Receiver tracks processed events:

```go
func (a *Actor) HandleEvent(ev gen.MessageEvent) error {
    switch m := ev.Message.(type) {
    case events.OrderCreatedV1:
        if a.state.processed[m.EventID] {
            return nil // duplicate, skip
        }
        a.state.processed[m.EventID] = true
        return a.handleOrderCreated(m)
    }
    return nil
}
```

EventID enables:
- Duplicate detection after redelivery
- Exactly-once processing semantics
- Debugging and tracing

## Contract Testing

[Contract tests](https://martinfowler.com/articles/microservice-testing/#testing-contract-introduction) verify that actors handle all supported versions:

```go
func TestPaymentActorAcceptsBothVersions(t *testing.T) {
    tc := unit.NewTestCase(t, "test@localhost")
    defer tc.Stop()

    actor := tc.Spawn(createPaymentActor)

    // V1 works
    actor.Send(ChargeRequestV1{OrderID: 1, Amount: 100})
    actor.ShouldSend().Message(ChargeResponseV1{}).Once().Assert()

    // V2 works
    actor.Send(ChargeRequestV2{OrderID: 2, Amount: 200, Currency: "EUR"})
    actor.ShouldSend().Message(ChargeResponseV2{}).Once().Assert()
}
```

Test ACL conversion:

```go
func TestACLConvertsV1ToV2(t *testing.T) {
    v1 := ChargeRequestV1{OrderID: 123, Amount: 500}
    v2 := acl.ChargeV1ToV2(v1)

    assert.Equal(t, v1.OrderID, v2.OrderID)
    assert.Equal(t, v1.Amount, v2.Amount)
    assert.Equal(t, "USD", v2.Currency) // default
}
```

Run contract tests in CI before merging changes to shared modules.

For actor testing patterns, see [Unit Testing](../testing/unit.md).

## Naming Conventions

### Async Messages

Prefix with `Message`:

```go
type MessageOrderShipped struct {
    OrderID   int64
    TrackingN string
}
```

The prefix signals fire-and-forget semantics. When reading code, `MessageXXX` means no response is expected. If someone writes `Call(pid, MessageOrderShipped{})`, the mismatch is immediately visible.

### Sync Messages

Use `Request`/`Response` suffix:

```go
type ChargeRequestV1 struct {
    OrderID int64
    Amount  int64
}

type ChargeResponseV1 struct {
    TransactionID string
    Status        string
}
```

Paired naming makes contracts explicit. `ChargeRequest` implies `ChargeResponse` exists. The caller knows to expect a result.

### Events

Domain events use past tense without prefix:

```go
type OrderCreatedV1 struct { ... }
type PaymentReceivedV1 struct { ... }
```

Events describe facts that already happened, not requests for action. Past tense (`Created`, `Received`) distinguishes them from commands (`Create`, `Charge`).

### Version Suffix

Always suffix with version number:

```go
type OrderV1 struct { ... }   // correct
type Order struct { ... }     // avoid - unclear versioning
type OrderNew struct { ... }  // avoid - not a version number
```

## Common Mistakes

**Changing existing type instead of creating new version**

```go
// Wrong - breaks existing consumers
type Order struct {
    ID       int64
    Priority int    // added field breaks wire format
}

// Correct - explicit new version
type OrderV2 struct {
    ID       int64
    Priority int
}
```

**Forgetting to register new types**

```go
// Type exists but not registered - encoding fails at runtime
type OrderV3 struct { ... }

// Must register before node starts
edf.RegisterTypeOf(OrderV3{})
```

**Long coexistence periods**

Supporting V1 for months creates maintenance burden. Set clear deprecation deadlines and enforce them.

**Missing EventID**

Without unique identifier, duplicate detection is impossible. Network retries cause duplicate processing.

**Registering after connection established**

Types must be registered before node starts. Dynamic registration requires connection cycling.

## Summary

| Aspect | Private Messages | Cluster Events |
|--------|------------------|----------------|
| Nature | Service API contract | Domain fact |
| Owner | Receiver (implements logic) | Shared (belongs to domain) |
| Location | `receiver-api/` module | `events/` module |
| Changes | Receiver team decides | All consumers coordinate |
| Versioning | Type suffix (V1, V2) | Type suffix (V1, V2) |

Key principles:
- Version in type name, not module path
- Receiver owns private contracts
- Shared repository for domain events
- Include EventID for idempotency
- Test serialization compatibility
- Set deprecation deadlines
- Use ACL to isolate version translation

