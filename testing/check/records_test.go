package check_test

import (
	"strings"
	"testing"

	"ergo.services/ergo/testing/check"
)

var allRecords = []check.Record{
	check.Send{}, check.Call{}, check.Spawn{}, check.RemoteSpawn{},
	check.RemoteApplicationStart{}, check.SpawnMeta{}, check.CreateAlias{},
	check.DeleteAlias{}, check.RegisterEvent{}, check.UnregisterEvent{},
	check.Forward{}, check.Delivered{}, check.Down{}, check.Exit{},
	check.Event{}, check.Monitor{}, check.Demonitor{}, check.Link{},
	check.Unlink{}, check.WireLink{}, check.WireUnlink{}, check.WireMonitor{},
	check.WireDemonitor{}, check.SendEvent{}, check.SendResponse{},
	check.SendExit{}, check.SendExitMeta{}, check.Span{}, check.Log{},
	check.AddCronJob{}, check.RemoveCronJob{}, check.Terminated{},
	check.SendAfter{}, check.SendEvery{},
}

func TestEveryRecordDescribesItself(t *testing.T) {
	kinds := map[string]bool{}
	for _, r := range allRecords {
		kind := r.Kind()
		if kind == "" {
			t.Errorf("%T reports an empty kind", r)
		}
		if kinds[kind] {
			t.Errorf("%T reuses the kind %q", r, kind)
		}
		kinds[kind] = true

		text := r.String()
		if text == "" {
			t.Errorf("%T renders as an empty string", r)
		}
		if strings.Contains(text, "%!") {
			t.Errorf("%T renders a broken format: %s", r, text)
		}
	}
}

func TestEveryRecordReachesTheRecorder(t *testing.T) {
	rec := check.NewRecorder()
	for _, r := range allRecords {
		rec.Put(r)
	}

	got := rec.Records()
	if len(got) != len(allRecords) {
		t.Fatalf("the recorder kept %d of %d records", len(got), len(allRecords))
	}
	for i, r := range got {
		if r.Kind() != allRecords[i].Kind() {
			t.Fatalf("record %d came back as %q, not %q", i, r.Kind(), allRecords[i].Kind())
		}
	}
}
