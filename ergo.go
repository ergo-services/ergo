package ergo

import (
	"context"
	"time"

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
	if opts.CloudEnable {
		cloudApp := cloud.CreateApp(opts.CloudOptions)
		opts.Applications = append([]gen.ApplicationBehavior{cloudApp}, opts.Applications...)
	}

	if opts.Handshake == nil {
		handshakeOptions := dist.HandshakeOptions{
			Cookie:  cookie,
			Version: dist.DefaultDistHandshakeVersion,
		}
		// set default handshake for the node (Erlang Dist Handshake)
		handshakeTimeout := 5 * time.Second
		opts.Handshake == dist.CreateHandshake(handshakeTimeout, handshakeOptions)
	}

	if opts.Proto == nil {
		// set default proto handler (Erlang Dist Proto)
		opts.Proto = dist.CreateProto(name, opts.Compression, opts.ProxyMode)
	}

	if opts.StaticRoutesOnly == false && opts.Resolver == nil {
		// set default resolver (Erlang EPMD service)
		enabledServer := ops.ResolverDisableServer == false
		opts.Resolver = dist.CreateResolver(ctx, enabledServer, opts.ResolverHost, opts.ResolverPort)
	}

	return node.StartWithContext(ctx, name, cookie, opts)
}
