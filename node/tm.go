package node

import "ergo.services/ergo/gen"

// tmBridge implements gen.CoreTargetManager interface
// It bridges TargetManager to node methods
type tmBridge struct {
	node *node
}

func createTMBridge(n *node) *tmBridge {
	return &tmBridge{
		node: n,
	}
}

// Name returns node name
func (b *tmBridge) Name() gen.Atom {
	return b.node.name
}

// PID returns node's CorePID
func (b *tmBridge) PID() gen.PID {
	return b.node.corePID
}

// Log returns node's logger
func (b *tmBridge) Log() gen.Log {
	return b.node.log
}

// MakeRef returns a unique reference within the node
func (b *tmBridge) MakeRef() gen.Ref {
	return b.node.MakeRef()
}

// RouteSendPID sends message to PID
func (b *tmBridge) RouteSendPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	return b.node.RouteSendPID(from, to, options, message)
}

// RouteSendExitMessages sends exit messages in batch
func (b *tmBridge) RouteSendExitMessages(from gen.PID, to []gen.PID, message any) error {
	for _, pid := range to {
		b.node.sendExitMessage(from, pid, message)
	}
	return nil
}

// RouteSendEventMessages sends event messages in batch (local only)
func (b *tmBridge) RouteSendEventMessages(from gen.PID, to []gen.PID, options gen.MessageOptions, message gen.MessageEvent) error {
	for _, pid := range to {
		b.node.sendEventMessage(from, pid, options.Priority, message)
	}
	return nil
}

// GetConnection returns network connection to remote node
func (b *tmBridge) GetConnection(node gen.Atom) (gen.Connection, error) {
	return b.node.network.GetConnection(node)
}

// IsAlivePID reports whether the process exists and is not terminated; the core is alive while the node runs
func (b *tmBridge) IsAlivePID(pid gen.PID) bool {
	if pid == b.node.corePID {
		return true
	}
	v, exist := b.node.processes.Load(pid)
	if exist == false {
		return false
	}
	return v.(*process).State() != gen.ProcessStateTerminated
}

// IsAliveProcessID reports whether the name belongs to a process that is not terminated
func (b *tmBridge) IsAliveProcessID(target gen.ProcessID) bool {
	v, exist := b.node.names.Load(target.Name)
	if exist == false {
		return false
	}
	return v.(*process).State() != gen.ProcessStateTerminated
}

// IsAliveAlias reports whether the alias is registered
func (b *tmBridge) IsAliveAlias(alias gen.Alias) bool {
	_, exist := b.node.aliases.Load(alias)
	return exist
}

// IsAliveNode reports whether the node is connected, without dialing it
func (b *tmBridge) IsAliveNode(node gen.Atom) bool {
	_, err := b.node.network.Connection(node)
	return err == nil
}
