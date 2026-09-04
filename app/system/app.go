package system

import (
	"ergo.services/ergo/app"
	"ergo.services/ergo/app/system/inspect"
	"ergo.services/ergo/app/system/manage"
	"ergo.services/ergo/gen"
)

const Name gen.Atom = "system_app"

// Options configures the system application.
type Options struct {
	// DisableManage keeps the mutating plane down. Without system_manage nothing can
	// change the node state through the system application, whatever rights the
	// caller has. Named negatively so the zero value keeps the plane up.
	DisableManage bool

	// ManagePoolSize is the worker count of the mutating plane.
	// Default: manage.DefaultPoolSize
	ManagePoolSize int
}

func CreateApp(options ...Options) gen.ApplicationBehavior {
	sa := &systemApp{}
	if len(options) > 0 {
		sa.options = options[0]
	}
	return sa
}

type systemApp struct {
	app.Application
	options Options
}

func (sa *systemApp) Load(args ...any) (gen.ApplicationSpec, error) {
	roles := map[string]gen.Atom{
		"inspect": inspect.Name,
	}
	env := map[gen.Env]any{
		inspect.EnvManage: sa.options.DisableManage == false,
	}

	if sa.options.DisableManage == false {
		roles["manage"] = manage.Name
		env[inspect.EnvManageProcess] = manage.Name
		env[inspect.EnvManageCapabilities] = manage.Capabilities()
		if sa.options.ManagePoolSize > 0 {
			env[manage.EnvPoolSize] = sa.options.ManagePoolSize
		}
	}

	return gen.ApplicationSpec{
		Name:        Name,
		Description: "System Application",
		Network: gen.ApplicationNetwork{
			// mutating types are registered either way: a caller reaching a node with
			// the plane down gets a clean refusal instead of a decoding failure
			RegisterTypes: append(inspect.Types(), manage.Types()...),
		},
		Env: env,
		Map: roles,
		Group: []gen.ApplicationMemberSpec{
			{
				Factory: factory_sup,
				Name:    "system_sup",
			},
		},
		Mode: gen.ApplicationModePermanent,
	}, nil
}
