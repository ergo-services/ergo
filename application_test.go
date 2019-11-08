package ergonode

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type MyApplication struct {
	Application
}

func (a *MyApplication) Load(args ...interface{}) (ApplicationSpec, error) {
	fmt.Println("Loading!!!")

	maxTime := args[0].(time.Duration)
	strategy := args[1].(string)
	return ApplicationSpec{
		Name:        "testapp",
		Description: "My Test Applicatoin",
		Environment: map[string]interface{}{
			"envName1": 123,
			"envName2": "Hello world",
		},
		Children: []ApplicationChildSpec{
			ApplicationChildSpec{},
		},
		MaxTime:  maxTime,
		Strategy: strategy,
	}, nil
}

func (a *MyApplication) Start(p *Process, args ...interface{}) {
	fmt.Println("STARTED!!!")
	p.SetEnv("MMM", 888)
}
func TestApplication(t *testing.T) {

	ctx := context.Background()
	node := CreateNodeWithContext(ctx, "nodeApplication@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}
	app := &MyApplication{}
	maxTime := 100 * time.Millisecond

	node.ApplicationLoad(app, maxTime, ApplicationStrategyPermanent)
	p, e := node.ApplicationStart("testapp")
	if e != nil {
		fmt.Println("ERR", e)
	}
	fmt.Println("PROC", p.Self())
	fmt.Println("XXX", p.ListEnv())

	p.SetEnv("ABB", 1.234)
	p.SetEnv("CDF", 567)
	p.SetEnv("GHJ", "890")

	fmt.Println("XXX", p.ListEnv())

	// node.ApplicationStart(app, maxTime, ApplicationStrategyTemporary)
	// node.ApplicationStart(app, maxTime, ApplicationStrategyTransient)

	node.Stop()
}
