package gen

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
