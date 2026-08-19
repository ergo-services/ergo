package system

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/app/system/inspect"
	"ergo.services/ergo/app/system/manage"
	"ergo.services/ergo/gen"
)

func factory_sup() gen.ProcessBehavior {
	return &sup{}
}

type sup struct {
	act.Supervisor
}

func (s *sup) Init(args ...any) (act.SupervisorSpec, error) {

	children := []act.SupervisorChildSpec{
		{
			Factory: inspect.Factory,
			Name:    inspect.Name,
		},
	}

	if enabled, _ := s.Env(inspect.EnvManage); enabled == true {
		children = append(children, act.SupervisorChildSpec{
			Factory: manage.Factory,
			Name:    manage.Name,
		})
	}

	spec := act.SupervisorSpec{
		Type:     act.SupervisorTypeOneForOne,
		Children: children,
	}
	spec.Restart.Strategy = act.SupervisorStrategyPermanent
	return spec, nil
}
