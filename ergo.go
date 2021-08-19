package ergo

import (
	"context"

	"github.com/halturin/ergo/node"
)

// StartNode create new node with name and cookie string
func StartNode(name string, cookie string, opts node.Options) (*node.Node, error) {
	return StartNodeWithContext(context.Background(), name, cookie, opts)
}

// CreateNodeWithContext create new node with specified context, name and cookie string
func StartNodeWithContext(ctx context.Context, name string, cookie string, opts node.Options) (*node.Node, error) {

	node := node.Start(ctx, name, cookie, opts)(*Node, error)
	node.Spawn("erlang_sup", ProcessOptions{}, &erlang.Sup{})

	return node, nil
}
