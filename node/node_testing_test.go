package node

import "ergo.services/ergo/gen"

// running is a node a unit test can call into. A node is usable when it has an
// incarnation and a state, and setting one of the two is how a test used to say
// this, which left it saying something the guards no longer read.
func running(n *node) *node {
	n.creation = 1
	n.state.Store(int32(gen.NodeStateRunning))
	return n
}
