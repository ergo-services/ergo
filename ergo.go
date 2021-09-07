package ergo

import (
	"context"

	"github.com/halturin/ergo/erlang"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
)

// StartNode create new node with name and cookie string
func StartNode(name string, cookie string, opts node.Options) (node.Node, error) {
	return StartNodeWithContext(context.Background(), name, cookie, opts)
}

// CreateNodeWithContext create new node with specified context, name and cookie string
func StartNodeWithContext(ctx context.Context, name string, cookie string, opts node.Options) (node.Node, error) {
	version := node.Version{
		Release: Version,
		Prefix:  VersionPrefix,
		OTP:     VersionOTP,
	}

	// add erlang support application
	opts.Applications = append([]gen.ApplicationBehavior{&erlang.KernelApp{}}, opts.Applications...)

	return node.StartWithContext(context.WithValue(ctx, "version", version), name, cookie, opts)
}
