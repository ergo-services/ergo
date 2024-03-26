package ergo

import (
	"runtime/debug"

	"ergo.services/ergo/app/system"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/node"
)

// StartNode starts a new node with given name
func StartNode(name gen.Atom, options gen.NodeOptions) (gen.Node, error) {
	var empty gen.Version

	if options.Version == empty {
		if info, ok := debug.ReadBuildInfo(); ok {
			options.Version.Name = info.Main.Path
			options.Version.Release = info.Main.Version
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					options.Version.Commit = setting.Value
					break
				}
			}
		}
	}

	// add default applications:
	defaultApps := []gen.ApplicationBehavior{
		system.CreateApp(),
	}

	options.Applications = append(defaultApps, options.Applications...)

	n, err := node.Start(name, options, FrameworkVersion)
	if err != nil {
		return nil, err
	}

	return n, nil
}
