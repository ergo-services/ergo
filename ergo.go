package ergo

import (
	"context"

	"github.com/ergo-services/ergo/erlang"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
	"github.com/ergo-services/ergo/proto/dist"
)

// StartNode create new node with name and cookie string
func StartNode(name string, cookie string, opts node.Options) (node.Node, error) {
	return StartNodeWithContext(context.Background(), name, cookie, opts)
}

// StartNodeWithContext create new node with specified context, name and cookie string
func StartNodeWithContext(ctx context.Context, name string, cookie string, opts node.Options) (node.Node, error) {
	version := node.Version{
		Release: Version,
		Prefix:  VersionPrefix,
		OTP:     VersionOTP,
	}

	// add erlang support application
	opts.Applications = append([]gen.ApplicationBehavior{&erlang.KernelApp{}}, opts.Applications...)

	if opts.CustomHandshake == nil {
		// set default handshake for the node (use Erlang Dist handshake)
		opts.CustomHandshake == dist.CreateDistHandshake()
	}

	return node.StartWithContext(context.WithValue(ctx, node.ContextKeyVersion, version), name, cookie, opts)
}
