package local

import (
	"fmt"
	"time"

	"ergo.services/ergo/gen"
)

var (
	errIncorrect = fmt.Errorf("incorrect")
)

type initcase struct{}

type testcase struct {
	name   string
	input  any
	output any
	err    chan error
}

func (t *testcase) wait(timeout int) error {
	timer := time.NewTimer(time.Second * time.Duration(timeout))
	defer timer.Stop()
	select {
	case <-timer.C:
		return gen.ErrTimeout
	case e := <-t.err:
		return e
	}
}
