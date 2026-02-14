package system

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/app/system/inspect"
	"ergo.services/ergo/gen"
)

func factory_sup() gen.ProcessBehavior {
	return &sup{}
}

type sup struct {
	act.Supervisor
}

func (s *sup) Init(args ...any) (act.SupervisorSpec, error) {

	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{
			{
				Factory: inspect.Factory,
				Name:    inspect.Name,
			},
		},
	}
	spec.Restart.Strategy = act.SupervisorStrategyPermanent
	return spec, nil
}
