package ergo

import (
	"fmt"
	"testing"

	"github.com/halturin/ergo/etf"
)

type GenStageTest struct {
	GenStage
}

func (gs *GenStageTest) InitStage(process *Process, args ...interface{}) (GenStageOptions, interface{}) {
	opts := GenStageOptions{
		stageType: GenStageTypeProducer,
		demand:    GenStageDemandModeForward,
	}
	return opts, nil
}

func (gs *GenStageTest) HandleCancel(cancelReason etf.Term, from etf.Term, state interface{}) (string, etf.Term, interface{}) {
	return "noreply", "subscription", state
}

func (gs *GenStageTest) HandleDemand(demand uint64, state interface{}) (string, etf.Term, interface{}) {
	return "noreply", 1, state
}

func (gs *GenStageTest) HandleEvents(events interface{}, from etf.Term, state interface{}) (string, interface{}) {
	return "asdf", state
}

func (gs *GenStageTest) HandleSubscribe(stageType GenStageType, options etf.List, state interface{}) (GenStageSubscriptionMode, interface{}) {
	return GenStageSubscriptionModeAuto, state
}

func TestGenStage(t *testing.T) {
	fmt.Printf("\n=== Test GenStage\n")
	fmt.Printf("Starting node: nodeGenStage01@localhost...")

	node1 := CreateNode("nodeGenStage01@localhost", "cookies", NodeOptions{})

	if node1 == nil {
		t.Fatal("can't start node")
		return
	}

	_, err := node1.Spawn("stage1", ProcessOptions{}, &GenStageTest{}, nil)

	fmt.Println("OK", err)

	node1.Stop()
}
