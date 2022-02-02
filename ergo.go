package ergo

import (
	"context"

	"github.com/ergo-services/ergo/cloud"
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

	// add cloud support if it's enabled
	if opts.Cloud.Enable {
		cloudApp := cloud.CreateApp(opts.Cloud)
		opts.Applications = append([]gen.ApplicationBehavior{cloudApp}, opts.Applications...)
	}

	if opts.Handshake == nil {
		handshakeOptions := dist.HandshakeOptions{
			Cookie: cookie,
		}
		// create default handshake for the node (Erlang Dist Handshake)
		opts.Handshake = dist.CreateHandshake(handshakeOptions)
	}

	if opts.Proto == nil {
		// create default proto handler (Erlang Dist Proto)
		protoOptions := node.DefaultProtoOptions()
		protoOptions.Compression = opts.Compression
		opts.Proto = dist.CreateProto(name, protoOptions)
	}

	if opts.StaticRoutesOnly == false && opts.Resolver == nil {
		// create default resolver (with enabled Erlang EPMD server)
		opts.Resolver = dist.CreateResolverWithLocalEPMD("", dist.DefaultEPMDPort)
	}

	return node.StartWithContext(ctx, name, cookie, opts)
}
