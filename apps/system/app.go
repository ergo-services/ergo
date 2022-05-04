package system

import (
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

type SystemApp struct {
	gen.Application
}

func (sa *SystemApp) Load(args ...etf.Term) (gen.ApplicationSpec, error) {
	lib.Log("SYSTEM: Application load")
	return gen.ApplicationSpec{
		Name:        "system_app",
		Description: "System Application",
		Version:     "v.1.0",
		Children: []gen.ApplicationChildSpec{
			gen.ApplicationChildSpec{
				Child: &systemAppSup{},
				Name:  "system_app_sup",
			},
		},
	}, nil
}

func (sa *SystemApp) Start(p gen.Process, args ...etf.Term) {
	lib.Log("SYSTEM: Application started")
}
