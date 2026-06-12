package distributed

import (
	"testing"

	"ergo.services/ergo/app"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// remApp is a minimal application that can be started remotely.
type remApp struct{ app.Application }

func createRemApp() gen.ApplicationBehavior { return &remApp{} }

func (a *remApp) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:  "rem_app",
		Group: []gen.ApplicationMemberSpec{{Factory: factorySpawnable}},
	}, nil
}

func containsAtom(list []gen.Atom, a gen.Atom) bool {
	for _, x := range list {
		if x == a {
			return true
		}
	}
	return false
}

// TestDistRemoteApp: a node starts and inspects an application on a peer through
// the RemoteNode handle. The app must be loaded and enabled on the peer; starting
// it moves it from Loaded to Running (observed via remote ApplicationInfo and the
// peer's ApplicationsRunning). Unknown names give ErrNameUnknown (start) and
// ErrApplicationUnknown (info).
func TestDistRemoteApp(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")

	appname, err := n2.Native().ApplicationLoad(createRemApp())
	check.NoError(t, err)
	n2.EnableApplicationStart(appname)
	remote := s.Connect(n1, n2)

	// remote info before start: loaded, not running
	info, err := remote.ApplicationInfo(appname)
	check.NoError(t, err)
	check.Equal(t, appname, info.Name)
	check.Equal(t, gen.ApplicationStateLoaded, info.State)
	check.Equal(t, false, containsAtom(n2.Native().ApplicationsRunning(), appname))

	// start it remotely
	check.NoError(t, remote.ApplicationStart(appname, gen.ApplicationOptions{}))
	check.Equal(t, true, containsAtom(n2.Native().ApplicationsRunning(), appname))

	// remote info after start: running
	info2, err := remote.ApplicationInfo(appname)
	check.NoError(t, err)
	check.Equal(t, gen.ApplicationStateRunning, info2.State)

	// unknown application
	check.True(t, remote.ApplicationStart("unknown", gen.ApplicationOptions{}) == gen.ErrNameUnknown)
	_, err = remote.ApplicationInfo("nonexistent")
	check.True(t, err == gen.ErrApplicationUnknown)
}
