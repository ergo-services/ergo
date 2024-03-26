package distributed

import (
	"fmt"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func createTestRemApp() gen.ApplicationBehavior {
	return &testremapp{}
}

type testremapp struct {
}

func (a *testremapp) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name: "test rem app",
		Group: []gen.ApplicationMemberSpec{
			{
				Name:    "test app member",
				Factory: factory_testappmember,
			},
		},
	}, nil
}

func (a *testremapp) Start(mode gen.ApplicationMode) {}
func (a *testremapp) Terminate(reason error)         {}

func factory_testappmember() gen.ProcessBehavior {
	return &testappmember{}
}

type testappmember struct {
	act.Actor
}

func TestT2RemoteAppStart(t *testing.T) {
	options1 := gen.NodeOptions{}
	options1.Network.Cookie = "123"
	options1.Network.MaxMessageSize = 567
	options1.Log.DefaultLogger.Disable = true
	// options1.Log.Level = gen.LogLevelTrace
	node1, err := ergo.StartNode("distT2node1remoteapp@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "123"
	options2.Network.MaxMessageSize = 765
	options2.Log.DefaultLogger.Disable = true
	node2, err := ergo.StartNode("distT2node2remoteapp@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	// make connection to the node2
	remoteNode2, err := node1.Network().GetNode(node2.Name())
	if err != nil {
		t.Fatal(err)
	}

	appname, err := node2.ApplicationLoad(createTestRemApp())
	if err != nil {
		t.Fatal(err)
	}

	node2.Network().EnableApplicationStart(appname)

	if err := remoteNode2.ApplicationStart(appname, gen.ApplicationOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := remoteNode2.ApplicationStart("unknown", gen.ApplicationOptions{}); err != gen.ErrNameUnknown {
		t.Fatal(fmt.Errorf("expected gen.ErrNameUnknown, got %q", err))
	}

}
