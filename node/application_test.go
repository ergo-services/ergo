package node

import (
	"strings"
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

// info().Uptime must be 0 for an application that never started (a.started == 0),
// not the full Unix epoch (now - 0).
func TestApplicationInfoUptimeNeverStarted(t *testing.T) {
	a := &application{node: &node{}}

	if u := a.info().Uptime; u != 0 {
		t.Fatalf("uptime for a never-started app = %d, want 0", u)
	}

	a.started = time.Now().Unix() - 5
	if u := a.info().Uptime; u < 5 || u > 60 {
		t.Fatalf("uptime for an app started ~5s ago = %d, want ~5", u)
	}
}

func TestApplicationDescribeCapsTheList(t *testing.T) {
	a := &application{node: &node{}}

	pids := make([]gen.PID, 0, logListLimit+15)
	for i := 0; i < cap(pids); i++ {
		pids = append(pids, gen.PID{Node: "n@localhost", ID: uint64(i)})
	}

	listed := a.describe(pids)
	if strings.Count(listed, "<") != logListLimit {
		t.Fatalf("describe listed %d process(es), want %d: %s",
			strings.Count(listed, "<"), logListLimit, listed)
	}
	if strings.HasSuffix(listed, ", ...and 15 more") == false {
		t.Fatalf("describe dropped 15 process(es) without saying so: %s", listed)
	}

	short := a.describe(pids[:3])
	if strings.Contains(short, "more") {
		t.Fatalf("describe added a tail to a list that fits: %s", short)
	}
	if strings.Count(short, "<") != 3 {
		t.Fatalf("describe listed %d of 3 process(es): %s", strings.Count(short, "<"), short)
	}

	if a.describe(nil) != "" {
		t.Fatalf("describe of nothing answered %q", a.describe(nil))
	}
}
