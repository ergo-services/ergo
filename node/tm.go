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
