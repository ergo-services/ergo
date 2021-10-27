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
	if opts.Env == nil {
		opts.Env = make(map[gen.EnvKey]interface{})
	}
	opts.Env[node.EnvKeyVersion] = version

	// add erlang support application
	opts.Applications = append([]gen.ApplicationBehavior{&erlang.KernelApp{}}, opts.Applications...)

	if opts.Handshake == nil {
		// set default handshake for the node (use Erlang Dist handshake)
		opts.Handshake == dist.CreateDistHandshake()
	}

	if opts.Proto == nil {
		// set default proto handler (Erlang Dist Proto)
		opts.Proto = dist.CreateDistProto()
	}

	return node.StartWithContext(ctx, name, cookie, opts)
}
