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

func (t *testcase) waitForCondition(timeout time.Duration, condition func() bool) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			if condition() {
				return
			}
		case <-timer.C:
			t.err <- fmt.Errorf("timeout waiting for condition after %s", timeout)
			return
		}
	}
}
