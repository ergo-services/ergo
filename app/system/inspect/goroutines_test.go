package inspect

import (
	"strings"
	"testing"
)

func TestNormalizeFuncLine(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "argument values are dropped",
			line: "ergo.services/ergo/node.(*node).Wait(0x5ab6aa6f14b0?)",
			want: "ergo.services/ergo/node.(*node).Wait(...)",
		},
		{
			name: "a method keeps its receiver",
			line: "ergo.services/ergo/net/proto.(*connection).wait(0x399a0fc9aa08)",
			want: "ergo.services/ergo/net/proto.(*connection).wait(...)",
		},
		{
			name: "braced arguments are dropped whole",
			line: "ergo.services/ergo/app/system/inspect.captureGoroutines({{0x0, 0x0}, {0x0, 0x0}, 0x0})",
			want: "ergo.services/ergo/app/system/inspect.captureGoroutines(...)",
		},
		{
			name: "several arguments",
			line: "internal/poll.(*FD).Read(0x399a0b81f180, {0x399a113e6000, 0x1000, 0x1000})",
			want: "internal/poll.(*FD).Read(...)",
		},
		{
			name: "an empty argument list is left alone",
			line: "ergo.services/ergo/node.(*node).SetCTRLC.func1()",
			want: "ergo.services/ergo/node.(*node).SetCTRLC.func1()",
		},
		{
			name: "the creator goroutine id is dropped",
			line: "created by ergo.services/ergo/net/proto.(*connection).Join in goroutine 3158709",
			want: "created by ergo.services/ergo/net/proto.(*connection).Join",
		},
		{
			name: "created by main.main has no id to drop",
			line: "main.main()",
			want: "main.main()",
		},
		{
			name: "elided frames are left alone",
			line: "internal/poll.(*pollDesc).waitRead(...)",
			want: "internal/poll.(*pollDesc).waitRead(...)",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizeFuncLine(c.line); got != c.want {
				t.Errorf("normalizeFuncLine(%q)\n got %q\nwant %q", c.line, got, c.want)
			}
		})
	}
}

func TestParseFuncLinesGroupsIdenticalCalls(t *testing.T) {
	first := strings.Join([]string{
		"goroutine 100 [chan receive, 5 minutes]:",
		"main.(*worker).loop(0x140001a2000)",
		"\t/path/worker.go:23 +0x1c",
		"created by main.start in goroutine 1",
		"\t/path/main.go:10 +0x40",
	}, "\n")
	second := strings.Join([]string{
		"goroutine 200 [chan receive]:",
		"main.(*worker).loop(0x14000999888)",
		"\t/path/worker.go:23 +0x1c",
		"created by main.start in goroutine 7",
		"\t/path/main.go:10 +0x40",
	}, "\n")

	a := strings.Join(parseFuncLines(first), "|")
	b := strings.Join(parseFuncLines(second), "|")
	if a != b {
		t.Errorf("the same call produced two keys:\n %s\n %s", a, b)
	}
	if strings.Contains(a, "0x") {
		t.Errorf("an address survived into the key: %s", a)
	}
	if strings.Contains(a, "goroutine") {
		t.Errorf("a goroutine id survived into the key: %s", a)
	}
}

func TestParseHeaderReadsLabelledGoroutine(t *testing.T) {
	block := strings.Join([]string{
		`goroutine 38669 [chan receive, 42 minutes] {pid: "<8B9E362E.0.1041>"}:`,
		"main.(*worker).loop(...)",
		"\t/path/worker.go:23 +0x1c",
	}, "\n")

	id, state, waitSec := parseHeader(block)
	if id != 38669 {
		t.Errorf("id %d, expected 38669", id)
	}
	if state != "chan receive" {
		t.Errorf("state %q, expected %q", state, "chan receive")
	}
	if waitSec != 42*60 {
		t.Errorf("wait %d seconds, expected %d", waitSec, 42*60)
	}
}

func TestParseWaitDuration(t *testing.T) {
	cases := map[string]int64{
		"1 minutes":        60,
		"42 minutes":       2520,
		"2 hours":          7200,
		"5 seconds":        5,
		"locked to thread": 0,
		"":                 0,
	}
	for in, want := range cases {
		if got := parseWaitDuration(in); got != want {
			t.Errorf("parseWaitDuration(%q) = %d, expected %d", in, got, want)
		}
	}
}

func TestCaptureGoroutines(t *testing.T) {
	all := captureGoroutines(RequestGetGoroutines{})
	if all.Error != nil {
		t.Fatalf("dump failed: %s", all.Error)
	}
	if all.Total == 0 {
		t.Fatal("a dump of this process counted no goroutines")
	}
	if all.Filtered != all.Total {
		t.Errorf("nothing was filtered, yet Filtered %d != Total %d", all.Filtered, all.Total)
	}
	if len(all.Groups) == 0 {
		t.Fatal("no groups")
	}
	if len(all.Groups) > all.Total {
		t.Errorf("%d groups over %d goroutines", len(all.Groups), all.Total)
	}

	counted := 0
	for _, g := range all.Groups {
		if g.Count != len(g.IDs) {
			t.Errorf("group of %d carries %d ids", g.Count, len(g.IDs))
		}
		if g.Stack == "" {
			t.Error("a group carries no stack")
		}
		counted += g.Count
	}
	if counted != all.Filtered {
		t.Errorf("groups hold %d goroutines, Filtered says %d", counted, all.Filtered)
	}

	waiting := captureGoroutines(RequestGetGoroutines{MinWait: 1})
	if waiting.Total != 0 && waiting.Filtered >= waiting.Total {
		t.Errorf("a wait filter kept everything: %d of %d", waiting.Filtered, waiting.Total)
	}
	for _, g := range waiting.Groups {
		if g.WaitSec == 0 {
			t.Errorf("group %q passed a wait filter with no wait", g.Current)
		}
	}

	none := captureGoroutines(RequestGetGoroutines{Stack: "no.such.function.anywhere"})
	if none.Filtered != 0 || len(none.Groups) != 0 {
		t.Errorf("a filter matching nothing returned %d goroutines in %d groups",
			none.Filtered, len(none.Groups))
	}
	if none.Total == 0 {
		t.Error("Total must count the whole dump even when the filter keeps nothing")
	}
}
