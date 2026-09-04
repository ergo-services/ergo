package distributed

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/app"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// MessageSetWeight tells the control member to change its application weight.
type MessageSetWeight struct{ W int }

// weightApp is a minimal application whose control member can change the
// application weight at runtime; the change propagates to the registrar.
type weightApp struct{ app.Application }

func createWeightApp() gen.ApplicationBehavior { return &weightApp{} }

func (a *weightApp) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:   "worker_app",
		Weight: 100,
		Mode:   gen.ApplicationModePermanent,
		Group: []gen.ApplicationMemberSpec{
			{Name: "worker_ctl", Factory: factoryWeightCtl},
		},
	}, nil
}

// weightCtl is the control member: on MessageSetWeight it updates the weight of
// its own application via Process.Application().SetWeight.
type weightCtl struct{ act.Actor }

func factoryWeightCtl() gen.ProcessBehavior { return &weightCtl{} }

func (w *weightCtl) HandleMessage(from gen.PID, message any) error {
	if m, ok := message.(MessageSetWeight); ok {
		return w.Application().SetWeight(m.W)
	}
	return nil
}

// resolveNodes resolves the application through the given resolver and returns
// the nodes still in rotation, retrying until the count matches want (weight
// changes reach the registrar asynchronously) or the deadline passes.
func resolveNodes(t *testing.T, r gen.Resolver, appName gen.Atom, want int) []gen.Atom {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		routes, err := r.ResolveApplication(appName)
		nodes := make([]gen.Atom, 0, len(routes))
		if err == nil {
			for _, route := range routes {
				nodes = append(nodes, route.Node)
			}
		}
		if len(nodes) == want {
			return nodes
		}
		if time.Now().After(deadline) {
			t.Fatalf("resolve %s: got %d nodes %v, want %d (err=%v)", appName, len(nodes), nodes, want, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestDistResolveWeightRotation: a negative application weight takes the
// instance out of the resolver's rotation cluster-wide; restoring a
// non-negative weight brings it back. Both transitions are driven through the
// real Application.SetWeight path on a running app.
func TestDistResolveWeightRotation(t *testing.T) {
	s := stage.New(t, stage.StageOptions{RegistrarFull: true})
	n1 := s.StartNode("n1", stage.NodeOptions{Applications: []gen.ApplicationBehavior{createWeightApp()}})
	n2 := s.StartNode("n2", stage.NodeOptions{Applications: []gen.ApplicationBehavior{createWeightApp()}})

	reg, err := n1.Native().Network().Registrar()
	check.NoError(t, err)
	resolver := reg.Resolver()

	// both instances registered with positive weight: both in rotation
	resolveNodes(t, resolver, "worker_app", 2)

	// take n2 out of rotation with a negative weight
	check.NoError(t, n2.Native().Send(gen.Atom("worker_ctl"), MessageSetWeight{W: -1}))
	nodes := resolveNodes(t, resolver, "worker_app", 1)
	check.Equal(t, n1.Name(), nodes[0])

	// restore n2: back in rotation
	check.NoError(t, n2.Native().Send(gen.Atom("worker_ctl"), MessageSetWeight{W: 5}))
	resolveNodes(t, resolver, "worker_app", 2)
}
