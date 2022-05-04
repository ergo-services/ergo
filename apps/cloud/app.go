package cloud

import (
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
)

type CloudApp struct {
	gen.Application
	options node.Cloud
}

func CreateApp(options node.Cloud) gen.ApplicationBehavior {
	return &CloudApp{
		options: options,
	}
}

func (ca *CloudApp) Load(args ...etf.Term) (gen.ApplicationSpec, error) {
	lib.Log("CLOUD_CLIENT: Application load")
	return gen.ApplicationSpec{
		Name:        "cloud_app",
		Description: "Ergo Cloud Support Application",
		Version:     "v.1.0",
		Children: []gen.ApplicationChildSpec{
			gen.ApplicationChildSpec{
				Child: &cloudAppSup{},
				Name:  "cloud_app_sup",
				Args:  []etf.Term{ca.options},
			},
		},
	}, nil
}

func (ca *CloudApp) Start(p gen.Process, args ...etf.Term) {
	lib.Log("CLOUD_CLIENT: Application started")
}
