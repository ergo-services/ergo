package system

import (
	"ergo.services/ergo/gen"
)

const Name gen.Atom = "system_app"

func CreateApp() gen.ApplicationBehavior {
	return &systemApp{}
}

type systemApp struct {
	node gen.Node
}

func (sa *systemApp) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
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

func (sa *systemApp) Start(mode gen.ApplicationMode) {}
func (sa *systemApp) Terminate(reason error)         {}
