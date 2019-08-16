package ergonode

// func (n *Node) Monitor(by etf.Pid, to etf.Pid) {
// 	var conn nodepeer
// 	var exists bool

// 	if string(to.Node) == n.FullName {
// 		lib.Log("Monitor local PID: %#v by %#v", to, by)

// 		pcs := n.channels[to]
// 		msg := []etf.Term{etf.Tuple{MONITOR, by, to, n.MakeRef()}}
// 		pcs.in <- msg

// 		return
// 	}

// 	lib.Log("Monitor remote PID: %#v by %#v", to, by)

// 	if conn, exists = n.peers[to.Node]; !exists {
// 		lib.Log("Send (via PID): create new connection (%s)", to.Node)
// 		if err := connect(n, to.Node); err != nil {
// 			panic(err.Error())
// 		}
// 		conn, _ = n.peers[to.Node]
// 	}

// 	msg := []etf.Term{etf.Tuple{MONITOR, by, to, n.MakeRef()}}
// 	conn.wchan <- msg
// }

// func (n *Node) MonitorNode(by etf.Pid, node etf.Atom, flag bool) {
// 	var exists bool
// 	var monitors []etf.Pid

// 	lib.Log("Monitor node: %#v by %#v", node, by)
// 	if _, exists = n.peers[node]; !exists {
// 		lib.Log("... connecting to %#v", node)
// 		if err := connect(n, node); err != nil {
// 			panic(err.Error())
// 		}
// 	}

// 	monitors = n.monitors[node]

// 	if !flag {
// 		lib.Log("... removing monitor: %#v by %#v", node, by)
// 		monitors = removePid(monitors, by)
// 	} else {
// 		lib.Log("... setting up monitor: %#v by %#v", node, by)
// 		// DUE TO...

// 		// http://erlang.org/doc/man/erlang.html#monitor_node-2
// 		// Making several calls to monitor_node(Node, true) for the same Node is not an error;
// 		// it results in as many independent monitoring instances.

// 		// DO NOT CHECK for existing this pid in the list, just add one more
// 		monitors = append(monitors, by)
// 	}

// 	n.monitors[node] = monitors
// 	lib.Log("Monitors for node (%#v): %#v", node, monitors)

// }

// func (n *Node) handle_monitors_node(node etf.Atom) {
// 	lib.Log("Node (%#v) is down. Send it to %#v", node, n.monitors[node])
// 	for _, pid := range n.monitors[node] {
// 		pcs := n.channels[pid]
// 		msg := etf.Term(etf.Tuple{etf.Atom("nodedown"), node})
// 		pcs.in <- msg
// 	}
// }
