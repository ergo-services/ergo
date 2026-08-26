package local

import (
	"fmt"
	"strings"
	"testing"

	"ergo.services/ergo/app/system/inspect"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// inspectGet asks the inspector of this node one of its one-shot requests.
func inspectGet(t *testing.T, n *stage.Node, request any) any {
	t.Helper()

	target := gen.ProcessID{Name: inspect.Name, Node: n.Name()}
	result, err := n.Native().Call(target, request)
	if err != nil {
		t.Fatalf("%T: %s", request, err)
	}
	return result
}

func typesOf(t *testing.T, n *stage.Node, request inspect.RequestGetTypes) inspect.ResponseGetTypes {
	t.Helper()

	response, ok := inspectGet(t, n, request).(inspect.ResponseGetTypes)
	if ok == false {
		t.Fatalf("unexpected response to %T", request)
	}
	check.NoError(t, response.Error)
	return response
}

// TestSystemInspectGetTypes: the registered types are answered filtered. A node registers
// hundreds of them, most of them the framework's own, so the whole list is a large answer to a
// small question and every filter here is what makes the question small.
func TestSystemInspectGetTypes(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{EnableSystemApp: true})

	all := typesOf(t, n, inspect.RequestGetTypes{})
	if len(all.Types) == 0 {
		t.Fatal("a node registers its own types, yet none came back")
	}
	if all.Truncated != 0 {
		t.Errorf("an unfiltered answer reports %d omitted", all.Truncated)
	}

	byName := typesOf(t, n, inspect.RequestGetTypes{Name: "PID"})
	if len(byName.Types) == 0 {
		t.Fatal("gen.PID is registered, so a name filter of PID must match something")
	}
	if len(byName.Types) >= len(all.Types) {
		t.Errorf("a name filter kept %d of %d types", len(byName.Types), len(all.Types))
	}
	for _, entry := range byName.Types {
		if strings.Contains(strings.ToLower(entry.Name), "pid") == false {
			t.Errorf("%q passed a name filter of PID", entry.Name)
		}
	}

	byKind := typesOf(t, n, inspect.RequestGetTypes{Kind: "struct"})
	if len(byKind.Types) == 0 {
		t.Fatal("no type of kind struct")
	}
	for _, entry := range byKind.Types {
		if strings.EqualFold(entry.Kind, "struct") == false {
			t.Errorf("%q is of kind %q and passed a filter of struct", entry.Name, entry.Kind)
		}
	}

	page := typesOf(t, n, inspect.RequestGetTypes{Limit: 3})
	if len(page.Types) != 3 {
		t.Fatalf("a page of three returned %d types", len(page.Types))
	}
	if page.Truncated != len(all.Types)-3 {
		t.Errorf("page omitted %d, expected %d", page.Truncated, len(all.Types)-3)
	}

	// a filter and a page together: the count of omitted is of what matched, not of the node
	pagedFilter := typesOf(t, n, inspect.RequestGetTypes{Kind: "struct", Limit: 2})
	if len(pagedFilter.Types) != 2 {
		t.Fatalf("a page of two returned %d types", len(pagedFilter.Types))
	}
	if pagedFilter.Truncated != len(byKind.Types)-2 {
		t.Errorf("page omitted %d of the %d matching, expected %d",
			pagedFilter.Truncated, len(byKind.Types), len(byKind.Types)-2)
	}

	none := typesOf(t, n, inspect.RequestGetTypes{Name: "no.such.type.anywhere"})
	if len(none.Types) != 0 || none.Truncated != 0 {
		t.Errorf("a filter matching nothing returned %d types and %d omitted",
			len(none.Types), none.Truncated)
	}
}

// TestSystemInspectGetProcessesTruncated: a listing says whether it is the whole set. Without
// that an answer of exactly the limit is indistinguishable from the whole node, and a caller
// that cannot see the cut reports a partial answer as complete.
func TestSystemInspectGetProcessesTruncated(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{EnableSystemApp: true})

	const spawned = 5
	for i := 0; i < spawned; i++ {
		n.SpawnRegister(gen.Atom(fmt.Sprintf("truncme_%d", i)), factoryEcho, gen.ProcessOptions{})
	}

	// the live map, unordered and cheap
	full, ok := inspectGet(t, n, inspect.RequestGetProcessRange{
		Name: "truncme", Limit: spawned + 1,
	}).(inspect.ResponseGetProcessRange)
	if ok == false {
		t.Fatal("unexpected response to RequestGetProcessRange")
	}
	check.NoError(t, full.Error)
	if len(full.Processes) != spawned {
		t.Fatalf("%d processes match, expected %d", len(full.Processes), spawned)
	}
	if full.Truncated {
		t.Error("a listing holding every match reports itself cut")
	}

	cut, _ := inspectGet(t, n, inspect.RequestGetProcessRange{
		Name: "truncme", Limit: 2,
	}).(inspect.ResponseGetProcessRange)
	check.NoError(t, cut.Error)
	if len(cut.Processes) != 2 {
		t.Fatalf("a page of two returned %d processes", len(cut.Processes))
	}
	if cut.Truncated == false {
		t.Error("a listing that left matches behind reports itself complete")
	}

	// exactly the limit, and nothing beyond it: the page is full and still complete
	exact, _ := inspectGet(t, n, inspect.RequestGetProcessRange{
		Name: "truncme", Limit: spawned,
	}).(inspect.ResponseGetProcessRange)
	check.NoError(t, exact.Error)
	if len(exact.Processes) != spawned {
		t.Fatalf("a page of %d returned %d processes", spawned, len(exact.Processes))
	}
	if exact.Truncated {
		t.Error("a page that happens to hold every match reports itself cut")
	}

	// the id space, ordered and repeatable
	ordered, ok := inspectGet(t, n, inspect.RequestGetProcessList{
		Name: "truncme", Limit: 2,
	}).(inspect.ResponseGetProcessList)
	if ok == false {
		t.Fatal("unexpected response to RequestGetProcessList")
	}
	check.NoError(t, ordered.Error)
	if len(ordered.Processes) != 2 {
		t.Fatalf("an ordered page of two returned %d processes", len(ordered.Processes))
	}
	if ordered.Truncated == false {
		t.Error("an ordered listing that left matches behind reports itself complete")
	}
	if ordered.Processes[0].PID.ID > ordered.Processes[1].PID.ID {
		t.Error("an ordered listing came back out of order")
	}

	orderedFull, _ := inspectGet(t, n, inspect.RequestGetProcessList{
		Name: "truncme", Limit: spawned + 1,
	}).(inspect.ResponseGetProcessList)
	check.NoError(t, orderedFull.Error)
	if len(orderedFull.Processes) != spawned {
		t.Fatalf("%d processes match, expected %d", len(orderedFull.Processes), spawned)
	}
	if orderedFull.Truncated {
		t.Error("an ordered listing holding every match reports itself cut")
	}

	// walking back from the newest returns the newest, not the oldest: the one process beyond
	// the page is the oldest of the set and must be the one dropped
	back, _ := inspectGet(t, n, inspect.RequestGetProcessList{
		Name: "truncme", Start: -1, Limit: 2,
	}).(inspect.ResponseGetProcessList)
	check.NoError(t, back.Error)
	if len(back.Processes) != 2 {
		t.Fatalf("a backward page of two returned %d processes", len(back.Processes))
	}
	if back.Truncated == false {
		t.Error("a backward listing that left matches behind reports itself complete")
	}
	newest := orderedFull.Processes[spawned-1].PID.ID
	if back.Processes[1].PID.ID != newest {
		t.Errorf("a backward page ends at %d, expected the newest %d",
			back.Processes[1].PID.ID, newest)
	}
}

// TestSystemInspectGetSubtree: the count of what the limit left out is exact, the same as the
// application tree reports. A bool there and a count here would be two meanings of one word in
// two neighbouring answers.
func TestSystemInspectGetSubtree(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{EnableSystemApp: true})

	sup := n.Spawn(factoryInspectSup, gen.ProcessOptions{})

	full, ok := inspectGet(t, n, inspect.RequestGetSubtree{PID: sup}).(inspect.ResponseGetSubtree)
	if ok == false {
		t.Fatal("unexpected response to RequestGetSubtree")
	}
	check.NoError(t, full.Error)

	// the supervisor of this suite runs two children
	if len(full.Processes) != 3 {
		t.Fatalf("the subtree holds %d processes, expected the supervisor and two children",
			len(full.Processes))
	}
	if full.Truncated != 0 {
		t.Errorf("a whole subtree reports %d omitted", full.Truncated)
	}
	if full.Processes[0].PID != sup {
		t.Errorf("the subtree starts at %s, expected its root %s", full.Processes[0].PID, sup)
	}

	root := inspectGet(t, n, inspect.RequestGetSubtree{PID: sup, Limit: 1})
	page, _ := root.(inspect.ResponseGetSubtree)
	check.NoError(t, page.Error)
	if len(page.Processes) != 1 {
		t.Fatalf("a page of one returned %d processes", len(page.Processes))
	}
	if page.Truncated != 2 {
		t.Errorf("a page of one omitted %d, expected the two children", page.Truncated)
	}

	unknown, _ := inspectGet(t, n, inspect.RequestGetSubtree{
		PID: gen.PID{Node: n.Name(), ID: 999999, Creation: 1},
	}).(inspect.ResponseGetSubtree)
	check.ErrorIs(t, unknown.Error, gen.ErrProcessUnknown)
}

// TestSystemInspectGetGoroutines: goroutines doing the same thing come back as one group. Every
// goroutine carries the addresses of its own arguments and the id of whoever created it, so
// without dropping those a node of a thousand actors answers with a thousand groups, and the
// answer is unreadable at exactly the size where it matters.
func TestSystemInspectGetGoroutines(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{EnableSystemApp: true})

	// a meta process parks a goroutine of its own in Start, so several of them are several
	// goroutines standing in one place
	const metas = 4
	for i := 0; i < metas; i++ {
		owner := n.Spawn(factoryMetaActor, gen.ProcessOptions{})
		if _, err := n.Call(owner, "spawnmeta"); err != nil {
			t.Fatalf("spawn meta: %s", err)
		}
	}

	out, ok := inspectGet(t, n, inspect.RequestGetGoroutines{
		Stack: "inspectMeta",
	}).(inspect.ResponseGetGoroutines)
	if ok == false {
		t.Fatal("unexpected response to RequestGetGoroutines")
	}
	check.NoError(t, out.Error)

	if out.Filtered < metas {
		t.Fatalf("%d goroutines match the metas, expected at least %d", out.Filtered, metas)
	}
	if out.Total < out.Filtered {
		t.Errorf("Total %d is below Filtered %d", out.Total, out.Filtered)
	}

	widest := 0
	for _, g := range out.Groups {
		if g.Count > widest {
			widest = g.Count
		}
	}
	if widest < metas {
		t.Errorf("%d goroutines standing in one place came back as groups of at most %d: %d groups",
			metas, widest, len(out.Groups))
	}
	if len(out.Groups) >= out.Filtered {
		t.Errorf("%d goroutines produced %d groups, so nothing was grouped",
			out.Filtered, len(out.Groups))
	}
}
