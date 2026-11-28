package gen

import "time"

// Standard message types published through the node-local CoreEvent system
// bus (the event named CoreEvent, owned by the node core). Unlike the
// registrar events, the CoreEvent bus is always available: it does not depend
// on networking or a registrar, and reports the lifecycle of this node only.
//
// Subscribe via process.LinkEvent / process.MonitorEvent on CoreEvent. The bus
// is buffered, so a subscriber that links after an event still receives the
// most recent transitions.

// MessageCoreApplicationStarted is published when an application on this node
// reached the running state.
type MessageCoreApplicationStarted struct {
	Name Atom
	Mode ApplicationMode
}

// MessageCoreApplicationStopped is published when a running application on this
// node stopped. Reason carries the termination reason that triggered the stop
// (for a permanent application, the reason of the member whose termination
// stopped it).
type MessageCoreApplicationStopped struct {
	Name   Atom
	Mode   ApplicationMode
	Reason error
}

// MessageCoreNodeStarted is published once the node has come up: the network,
// the registrar and the applications of NodeOptions.Applications are running,
// and StartNode is about to return. Applications started later through
// Node.ApplicationStart are not in it.
//
// It is the first message on this bus and so the first to be pushed out of it:
// a process that subscribes later will not see it. To ask what state the node
// is in rather than wait for the transition, use Node.State.
type MessageCoreNodeStarted struct {
	Applications []Atom
}

// MessageCoreNodeStopping is published when a graceful node shutdown begins, before
// any application is stopped. Drain is how long the node waits before stopping them.
// StopForce does not publish it.
type MessageCoreNodeStopping struct {
	Drain time.Duration
}

// MessageCoreNodeConnected is published when a connection with a remote node
// has been established.
type MessageCoreNodeConnected struct {
	Name Atom
}

// MessageCoreNodeDisconnected is published when a connection with a remote node
// has been lost. Reason carries the disconnect reason, if any.
type MessageCoreNodeDisconnected struct {
	Name   Atom
	Reason error
}
