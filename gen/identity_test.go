package gen

import (
	"encoding/json"
	"reflect"
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

// JSON is the machine channel: it carries the node name and the creation, and the
// value survives a round trip. Without that a receiver has to reconstruct the
// identity from context, which is only correct while everything is on one node.
func TestIdentityJSONRoundTrip(t *testing.T) {
	pid := PID{Node: "n@h", ID: 1 << 40, Creation: 1755000000}
	proc := ProcessID{Name: "worker", Node: "n@h"}
	ref := Ref{Node: "n@h", Creation: 1755000000, ID: [3]uint64{7, 8, 9}}
	alias := Alias{Node: "n@h", Creation: 1755000000, ID: [3]uint64{7, 8, 9}}
	ev := Event{Name: "tick", Node: "n@h"}

	roundTrip := func(name string, v any, back any) {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("%s marshal: %s", name, err)
		}
		if strings.Contains(string(data), "n@h") == false {
			t.Errorf("%s marshalled to %s, the node name is missing", name, data)
		}
		if err := json.Unmarshal(data, back); err != nil {
			t.Fatalf("%s unmarshal: %s", name, err)
		}
	}

	var pidBack PID
	roundTrip("PID", pid, &pidBack)
	if pidBack != pid {
		t.Errorf("PID came back as %#v, want %#v", pidBack, pid)
	}

	var procBack ProcessID
	roundTrip("ProcessID", proc, &procBack)
	if procBack != proc {
		t.Errorf("ProcessID came back as %#v, want %#v", procBack, proc)
	}

	var refBack Ref
	roundTrip("Ref", ref, &refBack)
	if refBack != ref {
		t.Errorf("Ref came back as %#v, want %#v", refBack, ref)
	}

	var aliasBack Alias
	roundTrip("Alias", alias, &aliasBack)
	if aliasBack != alias {
		t.Errorf("Alias came back as %#v, want %#v", aliasBack, alias)
	}

	var evBack Event
	roundTrip("Event", ev, &evBack)
	if evBack != ev {
		t.Errorf("Event came back as %#v, want %#v", evBack, ev)
	}
}

// Every enum that marshals to a name reads that name back. Without it a receiver
// hand-writes a parser per enum, which is where the names drift apart.
func TestEnumJSONRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		v    any
		back func() any
	}{
		{"ApplicationMode", ApplicationModePermanent, func() any { return new(ApplicationMode) }},
		{"ApplicationState", ApplicationStateRunning, func() any { return new(ApplicationState) }},
		{"MetaState", MetaStateTerminated, func() any { return new(MetaState) }},
		{"MetaState#", MetaState(42), func() any { return new(MetaState) }},
		{"CompressionLevel", CompressionBestSpeed, func() any { return new(CompressionLevel) }},
		{"NetworkMode", NetworkModeHidden, func() any { return new(NetworkMode) }},
		{"ProcessState", ProcessStateWaitResponse, func() any { return new(ProcessState) }},
		{"ProcessState#", ProcessState(99), func() any { return new(ProcessState) }},
		{"MessagePriority", MessagePriorityMax, func() any { return new(MessagePriority) }},
		{"LogLevel", LogLevelWarning, func() any { return new(LogLevel) }},
		{"LogLevelSystem", LogLevelSystem, func() any { return new(LogLevel) }},
		{"TracingPoint", TracingPointDelivered, func() any { return new(TracingPoint) }},
		{"TracingPoint#", TracingPoint(7), func() any { return new(TracingPoint) }},
		{"TracingKind", TracingKindSpawn, func() any { return new(TracingKind) }},
		{"TracingKindBusiness", TracingKind(0), func() any { return new(TracingKind) }},
	}

	for _, c := range cases {
		data, err := json.Marshal(c.v)
		if err != nil {
			t.Fatalf("%s marshal: %s", c.name, err)
		}
		back := c.back()
		if err := json.Unmarshal(data, back); err != nil {
			t.Fatalf("%s unmarshal %s: %s", c.name, data, err)
		}
		got := reflect.ValueOf(back).Elem().Interface()
		if got != c.v {
			t.Errorf("%s round-tripped %s into %v, want %v", c.name, data, got, c.v)
		}
	}
}

// an unknown name is an error, not a silent zero value
func TestEnumJSONUnknown(t *testing.T) {
	var level LogLevel
	if err := json.Unmarshal([]byte(`"warn"`), &level); err == nil {
		t.Error("an unknown log level was accepted")
	}

	var mode ApplicationMode
	if err := json.Unmarshal([]byte(`"forever"`), &mode); err == nil {
		t.Error("an unknown application mode was accepted")
	}
}

// The string-kind types carry their raw value in JSON. Their String methods are
// display forms - Atom quotes, Env uppercases - and json must not pick them up:
// encoding/json only looks for MarshalJSON and MarshalText, never Stringer.
func TestStringKindJSON(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want string
	}{
		{"Atom", Atom("demo@localhost"), `"demo@localhost"`},
		{"Env", Env("port"), `"port"`},
		{"CompressionType", CompressionTypeGZIP, `"` + string(CompressionTypeGZIP) + `"`},
	}
	for _, c := range cases {
		data, err := json.Marshal(c.v)
		if err != nil {
			t.Fatalf("%s marshal: %s", c.name, err)
		}
		if string(data) != c.want {
			t.Errorf("%s marshalled to %s, want %s", c.name, data, c.want)
		}
	}

	// as a map key too: the applications listing is keyed by Atom
	keyed, err := json.Marshal(map[Atom]int{"demo@localhost": 1})
	if err != nil {
		t.Fatalf("map marshal: %s", err)
	}
	if string(keyed) != `{"demo@localhost":1}` {
		t.Errorf("Atom key marshalled to %s", keyed)
	}

	var atom Atom
	if err := json.Unmarshal([]byte(`"demo@localhost"`), &atom); err != nil {
		t.Fatalf("atom unmarshal: %s", err)
	}
	if atom != "demo@localhost" {
		t.Errorf("Atom came back as %q", atom)
	}
}
