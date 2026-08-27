package distributed

import (
	"testing"
	"time"

	"ergo.services/ergo/app"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// versionedApp declares a version in its spec, the way a deployed application
// does, so a resolver can tell one rollout from another.
type versionedApp struct {
	app.Application
	version gen.Version
}

func createVersionedApp(version gen.Version) gen.ApplicationBehavior {
	return &versionedApp{version: version}
}

func (a *versionedApp) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:    "worker_app",
		Version: a.version,
		Weight:  10,
		Mode:    gen.ApplicationModePermanent,
		Group: []gen.ApplicationMemberSpec{
			{Name: "worker_ctl", Factory: factoryWeightCtl},
		},
	}, nil
}

// resolveVersions resolves the application and returns the version reported for
// each node, retrying until every route is in (registration is asynchronous).
func resolveVersions(t *testing.T, r gen.Resolver, appName gen.Atom, want int) map[gen.Atom]gen.Version {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		versions := make(map[gen.Atom]gen.Version)
		routes, err := r.ResolveApplication(appName)
		if err == nil {
			for _, route := range routes {
				versions[route.Node] = route.Version
			}
		}
		if len(versions) == want {
			return versions
		}
		if time.Now().After(deadline) {
			t.Fatalf("resolve %s: got %d route(s) %v, want %d (err=%v)", appName, len(versions), versions, want, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestDistResolveApplicationVersion: the version an application declares in its
// spec reaches the registrar with its route, so two nodes running different
// releases of the same application are distinguishable at resolve time.
func TestDistResolveApplicationVersion(t *testing.T) {
	old := gen.Version{Name: "worker_app", Release: "1.2.3"}
	next := gen.Version{Name: "worker_app", Release: "2.0.0"}

	s := stage.New(t, stage.StageOptions{RegistrarFull: true})
	n1 := s.StartNode("n1", stage.NodeOptions{Applications: []gen.ApplicationBehavior{createVersionedApp(old)}})
	n2 := s.StartNode("n2", stage.NodeOptions{Applications: []gen.ApplicationBehavior{createVersionedApp(next)}})

	reg, err := n1.Native().Network().Registrar()
	check.NoError(t, err)

	versions := resolveVersions(t, reg.Resolver(), "worker_app", 2)
	check.Equal(t, old, versions[n1.Name()])
	check.Equal(t, next, versions[n2.Name()])
}
