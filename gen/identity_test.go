package gen

import (
	"strings"
	"testing"
	"time"
)

func TestAtomHostAndString(t *testing.T) {
	if h := Atom("node1@localhost").Host(); h != "localhost" {
		t.Fatalf("Host = %q, want localhost", h)
	}
	if h := Atom("noatsign").Host(); h != "" {
		t.Fatalf("Host of a hostless atom = %q, want empty", h)
	}
	if s := Atom("worker").String(); s != "'worker'" {
		t.Fatalf("Atom.String = %q, want 'worker'", s)
	}
}

func TestRefIsAliveAndDeadline(t *testing.T) {
	now := uint64(time.Now().Unix())

	if (Ref{ID: [3]uint64{1, 2, 0}}).IsAlive() == false {
		t.Fatal("Ref with a zero deadline should be alive")
	}
	if (Ref{ID: [3]uint64{1, 2, now + 60}}).IsAlive() == false {
		t.Fatal("Ref with a future deadline should be alive")
	}
	past := Ref{ID: [3]uint64{1, 2, now - 60}}
	if past.IsAlive() {
		t.Fatal("Ref with a past deadline should not be alive")
	}
	if past.Deadline() != now-60 {
		t.Fatalf("Deadline = %d, want %d", past.Deadline(), now-60)
	}
}

// String stays the compact display form for logs: the node is a CRC32 and a PID
// drops its creation.
func TestIdentityString(t *testing.T) {
	cases := []struct {
		name   string
		str    string
		prefix string
		substr string
	}{
		{"PID", PID{Node: "n@h", ID: 5, Creation: 1}.String(), "<", "5"},
		{"ProcessID", ProcessID{Name: "worker", Node: "n@h"}.String(), "<", "worker"},
		{"Ref", Ref{Node: "n@h", ID: [3]uint64{7, 8, 9}}.String(), "Ref#<", "7"},
		{"Alias", Alias{Node: "n@h", ID: [3]uint64{7, 8, 9}}.String(), "Alias#<", "7"},
		{"Event", Event{Name: "tick", Node: "n@h"}.String(), "Event#<", "tick"},
	}
	for _, c := range cases {
		if strings.HasPrefix(c.str, c.prefix) == false {
			t.Errorf("%s.String = %q, want prefix %q", c.name, c.str, c.prefix)
		}
		if strings.Contains(c.str, c.substr) == false {
			t.Errorf("%s.String = %q, want to contain %q", c.name, c.str, c.substr)
		}
	}
}
