package local

import (
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

//
// this is the template for writing new tests
//

var (
	tXXXcases []*testcase
)

func factory_tXXX() gen.ProcessBehavior {
	return &tXXX{}
}

type tXXX struct {
	act.Actor

	testcase *testcase
}

func (t *tXXX) HandleMessage(from gen.PID, message any) error {
	if t.testcase == nil {
		t.testcase = message.(*testcase)
		message = initcase{}
	}
	// get method by name
	method := reflect.ValueOf(t).MethodByName(t.testcase.name)
	if method.IsValid() == false {
		t.testcase.err <- fmt.Errorf("unknown method %q", t.testcase.name)
		t.testcase = nil
		return nil
	}
	method.Call([]reflect.Value{reflect.ValueOf(message)})
	return nil
}

func (t *tXXX) TestFeature1(input any) {
	defer func() {
		t.testcase = nil
	}()

	t.testcase.err <- nil
}

func (t *tXXX) TestFeatureX(input any) {
	defer func() {
		t.testcase = nil
	}()

	t.testcase.err <- nil
}

func TestTXXXtemplate(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	//nopt.Log.Level = gen.LogLevelTrace
	node, err := ergo.StartNode("tXXXnode@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_tXXX, popt)
	if err != nil {
		panic(err)
	}

	tXXXcases = []*testcase{
		&testcase{"TestFeature1", nil, nil, make(chan error)},
		&testcase{"TestFeatureX", nil, nil, make(chan error)},
	}
	for _, tc := range tXXXcases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(3); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
