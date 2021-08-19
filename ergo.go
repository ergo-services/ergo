package ergo

import (
	"context"

	"github.com/halturin/ergo/node"
)

// StartNode create new node with name and cookie string
func StartNode(name string, cookie string, opts node.Options) (node.Node, error) {
	return StartNodeWithContext(context.Background(), name, cookie, opts)
}

// CreateNodeWithContext create new node with specified context, name and cookie string
func StartNodeWithContext(ctx context.Context, name string, cookie string, opts node.Options) (node.Node, error) {
	versions := map[string]interface{}{
		"version": Version,
		"prefix":  VersionPrefix,
		"otp":     VersionOTP,
	}

	node, err := node.StartWithContext(context.WithValue(ctx, "versions", versions), name, cookie, opts)
	if err != nil {
		return nil, err
	}
	//node.Spawn("erlang_sup", gen.ProcessOptions{}, &erlang.Sup{})

	return node, nil
}
