package act

import (
	"reflect"
	"testing"

	"ergo.services/ergo/gen"
)

func Test_sortSupChild(t *testing.T) {
	node := gen.Atom("node1@localhost")
	spec1 := supChildSpec{
		i: 30,
	}
	spec1.Name = "s1"
	spec2 := supChildSpec{
		i: 20,
	}
	spec2.Name = "s2"
	spec3 := supChildSpec{
		i:        25,
		register: true,
	}
	spec3.Name = "s3"

	data := []supChild{
		{
			pid:  gen.PID{Node: node, ID: 1018},
			spec: spec1,
		},
		{
			pid:  gen.PID{Node: node, ID: 1014},
			spec: spec2,
		},
		{
			pid:  gen.PID{Node: node, ID: 1044},
			spec: spec3,
		},
		{
			pid:  gen.PID{Node: node, ID: 1013},
			spec: spec1,
		},
		{
			pid:  gen.PID{Node: node, ID: 1024},
			spec: spec2,
		},
		{
			pid:  gen.PID{Node: node, ID: 1019},
			spec: spec2,
		},
	}

	children := sortSupChild(data)
	expected := []SupervisorChild{
		{"s2", "", gen.PID{Node: node, ID: 1014}, false, false},
		{"s2", "", gen.PID{Node: node, ID: 1019}, false, false},
		{"s2", "", gen.PID{Node: node, ID: 1024}, false, false},
		{"s3", "s3", gen.PID{Node: node, ID: 1044}, false, false},
		{"s1", "", gen.PID{Node: node, ID: 1013}, false, false},
		{"s1", "", gen.PID{Node: node, ID: 1018}, false, false},
	}
	if reflect.DeepEqual(children, expected) == false {
		t.Fatal("mismatch")
	}
}
