package gen

import (
	"encoding/json"
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

// every identity type formats with a recognizable prefix and marshals to JSON as
// its quoted String().
func TestIdentityStringAndJSON(t *testing.T) {
	pid := PID{Node: "n@h", ID: 5, Creation: 1}
	proc := ProcessID{Name: "worker", Node: "n@h"}
	ref := Ref{Node: "n@h", ID: [3]uint64{7, 8, 9}}
	alias := Alias{Node: "n@h", ID: [3]uint64{7, 8, 9}}
	ev := Event{Name: "tick", Node: "n@h"}

	cases := []struct {
		name   string
		str    string
		v      json.Marshaler
		prefix string
		substr string
	}{
		{"PID", pid.String(), pid, "<", "5"},
		{"ProcessID", proc.String(), proc, "<", "worker"},
		{"Ref", ref.String(), ref, "Ref#<", "7"},
		{"Alias", alias.String(), alias, "Alias#<", "7"},
		{"Event", ev.String(), ev, "Event#<", "tick"},
	}
	for _, c := range cases {
		if strings.HasPrefix(c.str, c.prefix) == false {
			t.Errorf("%s.String = %q, want prefix %q", c.name, c.str, c.prefix)
		}
		if strings.Contains(c.str, c.substr) == false {
			t.Errorf("%s.String = %q, want to contain %q", c.name, c.str, c.substr)
		}
		js, err := c.v.MarshalJSON()
		if err != nil {
			t.Fatalf("%s MarshalJSON: %s", c.name, err)
		}
		if string(js) != `"`+c.str+`"` {
			t.Errorf("%s MarshalJSON = %s, want the quoted String", c.name, js)
		}
	}
}
