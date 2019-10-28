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

func (a *MyApplication) Start(p Process, args ...interface{}) ApplicationSpec {
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
	}
}
func TestApplication(t *testing.T) {

	ctx := context.Background()
	node := CreateNodeWithContext(ctx, "nodeApplication@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}
	app := &MyApplication{
		Application{},
	}
	maxTime := 100 * time.Millisecond
	node.ApplicationStart(app, maxTime, ApplicationStrategyPermanent)

	x := app.ListEnv()
	fmt.Println("XXX", x)

	// node.ApplicationStart(app, maxTime, ApplicationStrategyTemporary)
	// node.ApplicationStart(app, maxTime, ApplicationStrategyTransient)

	node.Stop()
}
