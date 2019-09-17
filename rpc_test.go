package ergonode

// start node1 with RPC
//  start n1gs1

//  start node2 with disabled RPC
//  start n2gs1

//  register func1 as node1.func1 method

//  rpc call n2gs1.call(node1.func1) (receive reply)
//  rpc call n1gs1.call(node2.whatevername) (recekve badrpc error)

import (
	"testing"
)

func TestRPC(t *testing.T) {
	// node := CreateNode("node@localhost","cookies", NodeOptions{})
}
