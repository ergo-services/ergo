package distributed

import (
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func createTestRemAppInfo() gen.ApplicationBehavior {
	return &testremappinfo{}
}

type testremappinfo struct {
}

func (a *testremappinfo) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:        "test_app_info",
		Description: "Test Application for Info",
		Version:     gen.Version{Name: "1.0.0", Release: "test"},
		Tags:        []gen.Atom{"test", "info"},
		Group: []gen.ApplicationMemberSpec{
			{
				Name:    "member1",
				Factory: factory_testappinfomember,
			},
			{
				Name:    "member2",
				Factory: factory_testappinfomember,
			},
		},
	}, nil
}

func (a *testremappinfo) Start(mode gen.ApplicationMode) {}
func (a *testremappinfo) Terminate(reason error)         {}

func factory_testappinfomember() gen.ProcessBehavior {
	return &testappinfomember{}
}

type testappinfomember struct {
	act.Actor
}

func TestT2RemoteAppInfo(t *testing.T) {
	options1 := gen.NodeOptions{}
	options1.Network.Cookie = "test123"
	options1.Log.DefaultLogger.Disable = true
	node1, err := ergo.StartNode("distT2node1appinfo@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "test123"
	options2.Log.DefaultLogger.Disable = true
	node2, err := ergo.StartNode("distT2node2appinfo@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	// Make connection to node2
	remoteNode2, err := node1.Network().GetNode(node2.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Load application on node2
	appname, err := node2.ApplicationLoad(createTestRemAppInfo())
	if err != nil {
		t.Fatal(err)
	}
	defer node2.ApplicationUnload(appname)

	// Test 1: Get info for loaded but not started application
	info, err := remoteNode2.ApplicationInfo(appname)
	if err != nil {
		t.Fatalf("ApplicationInfo failed: %s", err)
	}

	if info.Name != appname {
		t.Fatalf("expected app name %q, got %q", appname, info.Name)
	}
	if info.State != gen.ApplicationStateLoaded {
		t.Fatalf("expected state Loaded, got %v", info.State)
	}
	if info.Description != "Test Application for Info" {
		t.Fatalf("expected description 'Test Application for Info', got %q", info.Description)
	}

	// Start the application
	node2.Network().EnableApplicationStart(appname)
	if err := remoteNode2.ApplicationStart(appname, gen.ApplicationOptions{}); err != nil {
		t.Fatal(err)
	}

	// Test 2: Get info for running application
	info2, err := remoteNode2.ApplicationInfo(appname)
	if err != nil {
		t.Fatalf("ApplicationInfo after start failed: %s", err)
	}

	if info2.Name != appname {
		t.Fatalf("expected app name %q, got %q", appname, info2.Name)
	}
	if info2.State != gen.ApplicationStateRunning {
		t.Fatalf("expected state Running, got %v", info2.State)
	}
	if len(info2.Group) != 2 {
		t.Fatalf("expected 2 group members, got %d", len(info2.Group))
	}
	if len(info2.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(info2.Tags))
	}

	// Test 3: Query non-existent application
	_, err = remoteNode2.ApplicationInfo("nonexistent_app")
	if err != gen.ErrApplicationUnknown {
		t.Fatalf("expected ErrApplicationUnknown, got %v", err)
	}
}
