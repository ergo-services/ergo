package gen

import (
	"encoding/json"
	"testing"
)

// A numbered form within range round-trips through the named-set fallback.
func TestUnmarshalNumberedInRange(t *testing.T) {
	var ms MetaState
	if err := json.Unmarshal([]byte(`"state#7"`), &ms); err != nil {
		t.Fatal(err)
	}
	if ms != MetaState(7) {
		t.Fatalf("MetaState: got %d, want 7", int32(ms))
	}

	var ps ProcessState
	if err := json.Unmarshal([]byte(`"state#99"`), &ps); err != nil {
		t.Fatal(err)
	}
	if ps != ProcessState(99) {
		t.Fatalf("ProcessState: got %d, want 99", int32(ps))
	}

	var tp TracingPoint
	if err := json.Unmarshal([]byte(`"point#42"`), &tp); err != nil {
		t.Fatal(err)
	}
	if tp != TracingPoint(42) {
		t.Fatalf("TracingPoint: got %d, want 42", int(tp))
	}

	var tk TracingKind
	if err := json.Unmarshal([]byte(`"kind#42"`), &tk); err != nil {
		t.Fatal(err)
	}
	if tk != TracingKind(42) {
		t.Fatalf("TracingKind: got %d, want 42", int(tk))
	}
}

// A numbered form beyond the 32-bit range is rejected, not truncated into a
// different (and valid-looking) value.
func TestUnmarshalNumberedOutOfRange(t *testing.T) {
	// 4294967298 truncates to 2, 4294967297 to 1: both name a real state
	inputs := []struct {
		name  string
		json  string
		apply func(data []byte) error
	}{
		{"MetaState", `"state#4294967298"`, func(data []byte) error {
			var v MetaState
			return json.Unmarshal(data, &v)
		}},
		{"ProcessState", `"state#4294967297"`, func(data []byte) error {
			var v ProcessState
			return json.Unmarshal(data, &v)
		}},
		{"TracingPoint", `"point#4294967298"`, func(data []byte) error {
			var v TracingPoint
			return json.Unmarshal(data, &v)
		}},
		{"TracingKind", `"kind#4294967298"`, func(data []byte) error {
			var v TracingKind
			return json.Unmarshal(data, &v)
		}},
	}

	for _, in := range inputs {
		t.Run(in.name, func(t *testing.T) {
			if err := in.apply([]byte(in.json)); err == nil {
				t.Fatalf("%s accepted an out-of-range numbered form %s", in.name, in.json)
			}
		})
	}
}

// Every state that String names round-trips through MarshalJSON/UnmarshalJSON.
func TestStateJSONRoundTrip(t *testing.T) {
	metas := []MetaState{MetaStateSleep, MetaStateRunning, MetaStateTerminated, MetaState(31)}
	for _, want := range metas {
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var got MetaState
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("%s: %v", data, err)
		}
		if got != want {
			t.Fatalf("MetaState round-trip: got %d, want %d", int32(got), int32(want))
		}
	}

	procs := []ProcessState{
		ProcessStateInit,
		ProcessStateSleep,
		ProcessStateRunning,
		ProcessStateWaitResponse,
		ProcessStateTerminated,
		ProcessStateZombee,
		ProcessState(127),
	}
	for _, want := range procs {
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var got ProcessState
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("%s: %v", data, err)
		}
		if got != want {
			t.Fatalf("ProcessState round-trip: got %d, want %d", int32(got), int32(want))
		}
	}
}
