package gen

import (
	"fmt"

	"github.com/halturin/ergo/etf"
)

type SagaWorkerSup struct {
	Supervisor
}

func (ws *SagaWorkerSup) Init(args ...etf.Term) (SupervisorSpec, error) {
	worker, is_worker := args[0].(SagaWorkerBehavior)
	if !is_worker {
		return SupervisorSpec{}, fmt.Errorf("Not a gen.SagaWorkerBehavior")
	}
	return SupervisorSpec{
		Name: "gen_saga_worker_sup",
		Children: []SupervisorChildSpec{
			SupervisorChildSpec{
				Name:  "gen_saga_worker",
				Child: worker,
			},
		},
		Strategy: SupervisorStrategy{
			Type:      SupervisorStrategySimpleOneForOne,
			Intensity: 5,
			Period:    5,
			Restart:   SupervisorStrategyRestartTemporary,
		},
	}, nil
}
