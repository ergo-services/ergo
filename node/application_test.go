package node

import (
	"testing"
	"time"
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
