package local

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// TestLocalNodeProcessListShortInfoBounds: the walk over the id space stops at the end of it.
// The bound used to be compared by equality, so a start above the newest id never reached it
// going up and the walk ran to the end of int64 - inside a caller's callback, which is where
// the inspector calls this from. The deadline here is what tells that hang apart from a
// mistaken answer: without it the whole suite waits for its own timeout.
func TestLocalNodeProcessListShortInfoBounds(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()

	const spawned = 4
	for i := 0; i < spawned; i++ {
		n.SpawnRegister(gen.Atom(fmt.Sprintf("listed_%d", i)), factoryEcho, gen.ProcessOptions{})
	}

	type answer struct {
		list []gen.ProcessShortInfo
		err  error
	}

	// beyond the newest id there is nothing to walk over, and saying so must be immediate
	done := make(chan answer, 1)
	go func() {
		list, err := nd.ProcessListShortInfo(1<<40, 100)
		done <- answer{list: list, err: err}
	}()
	select {
	case got := <-done:
		check.NoError(t, got.err)
		if len(got.list) != 0 {
			t.Errorf("a start beyond the newest id returned %d processes", len(got.list))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a start beyond the newest id never came back")
	}

	listed := func(info gen.ProcessShortInfo) bool {
		return strings.HasPrefix(string(info.Name), "listed_")
	}

	// the id space starts at 1000, which is where a forward walk starts
	forward, err := nd.ProcessListShortInfo(1000, 1000, listed)
	check.NoError(t, err)
	if len(forward) != spawned {
		t.Fatalf("%d of the spawned processes came back, expected %d", len(forward), spawned)
	}

	// walking back from the newest returns the newest
	backward, err := nd.ProcessListShortInfo(-1, 1, listed)
	check.NoError(t, err)
	if len(backward) != 1 {
		t.Fatalf("a backward page of one returned %d processes", len(backward))
	}
	if backward[0].PID.ID != forward[spawned-1].PID.ID {
		t.Errorf("walking back gave %d, expected the newest %d",
			backward[0].PID.ID, forward[spawned-1].PID.ID)
	}

	// a start below the first id is not a start, and a negative limit is not a page
	_, err = nd.ProcessListShortInfo(10, 10)
	check.ErrorIs(t, err, gen.ErrIncorrect)
	_, err = nd.ProcessListShortInfo(1000, -1)
	check.ErrorIs(t, err, gen.ErrIncorrect)
}
