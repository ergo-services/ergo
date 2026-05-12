package system

import (
	"fmt"

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
	if err := inspect.RegisterTypes(sa.Node().Network()); err != nil {
		return gen.ApplicationSpec{}, fmt.Errorf("inspect types: %w", err)
	}
	return gen.ApplicationSpec{
		Name:        Name,
		Description: "System Application",
		Group: []gen.ApplicationMemberSpec{
			{
				Factory: factory_sup,
				Name:    "system_sup",
			},
		},
		Mode: gen.ApplicationModePermanent,
	}, nil
}
