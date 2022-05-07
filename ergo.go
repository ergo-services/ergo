package ergo

import (
	"context"

	"github.com/ergo-services/ergo/apps/cloud"
	"github.com/ergo-services/ergo/apps/erlang"
	"github.com/ergo-services/ergo/apps/system"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
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

	// add default applications:
	defaultApps := []gen.ApplicationBehavior{
		system.CreateApp(opts.System), // system application (bus, metrics etc.)
		erlang.CreateApp(),            // erlang support
	}

	// add cloud support if it's enabled
	if opts.Cloud.Enable {
		cloudApp := cloud.CreateApp(opts.Cloud)
		defaultApps = append(defaultApps, cloudApp)
		if opts.Proxy.Accept == false {
			lib.Warning("Disabled option Proxy.Accept makes this node inaccessible to the other nodes within your cloud cluster, but it still allows initiate connection to the others with this option enabled.")
		}
	}
	opts.Applications = append(defaultApps, opts.Applications...)

	if opts.Handshake == nil {
		// create default handshake for the node (Erlang Dist Handshake)
		opts.Handshake = dist.CreateHandshake(dist.HandshakeOptions{})
	}

	if opts.Proto == nil {
		// create default proto handler (Erlang Dist Proto)
		protoOptions := node.DefaultProtoOptions()
		opts.Proto = dist.CreateProto(protoOptions)
	}

	if opts.StaticRoutesOnly == false && opts.Resolver == nil {
		// create default resolver (with enabled Erlang EPMD server)
		opts.Resolver = dist.CreateResolverWithLocalEPMD("", dist.DefaultEPMDPort)
	}

	return node.StartWithContext(ctx, name, cookie, opts)
}
