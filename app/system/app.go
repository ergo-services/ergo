package system

import (
	"ergo.services/ergo/app"
	"ergo.services/ergo/app/system/inspect"
	"ergo.services/ergo/gen"
)

const Name gen.Atom = "system_app"

func CreateApp() gen.ApplicationBehavior {
	return &systemApp{}
}

type systemApp struct {
	app.Application
}

func (sa *systemApp) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:        Name,
		Description: "System Application",
		Network: gen.ApplicationNetwork{
			RegisterTypes: inspect.Types(),
		},
		Group: []gen.ApplicationMemberSpec{
			{
				Factory: factory_sup,
				Name:    "system_sup",
			},
		},
		Mode: gen.ApplicationModePermanent,
	}, nil
}
