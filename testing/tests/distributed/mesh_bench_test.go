package distributed

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"ergo.services/ergo/testing/stage"
)

// TestMeshBench measures mesh-formation time and reconnection churn across a grid of
// cluster sizes and pool depths. It reports two timings per cell: conn(ms), when every
// ordered pair is connected (its primary TCP up, one link per pair), and form(ms), when
// every pair has filled its TCP pool. The delta is the cost of pool fill over basic
// connectivity. Gated behind ERGO_MESH_BENCH so it never runs in the normal suite. Run
// one config per fresh process to avoid TIME_WAIT accumulation skewing the larger configs:
//
//	ERGO_MESH_BENCH=1 ERGO_MESH_N=50 ERGO_MESH_POOL=10 go test -run '^TestMeshBench$' -timeout 600s -v ./testing/tests/distributed/
func TestMeshBench(t *testing.T) {
	if os.Getenv("ERGO_MESH_BENCH") == "" {
		t.Skip("set ERGO_MESH_BENCH=1 to run the mesh formation benchmark")
	}

	Ns := []int{10, 20, 30, 50}
	Pools := []int{3, 5, 10}
	if v := os.Getenv("ERGO_MESH_N"); v != "" {
		var n int
		fmt.Sscanf(v, "%d", &n)
		Ns = []int{n}
	}
	if v := os.Getenv("ERGO_MESH_POOL"); v != "" {
		var p int
		fmt.Sscanf(v, "%d", &p)
		Pools = []int{p}
	}

	type row struct {
		n, pool        int
		connMS, formMS float64
		links          int
		reconn         uint64
	}
	var rows []row

	for _, n := range Ns {
		for _, pool := range Pools {
			t.Run(fmt.Sprintf("N%d_pool%d", n, pool), func(t *testing.T) {
				connMS, formMS, reconn, links := benchOne(t, n, pool)
				rows = append(rows, row{n, pool, connMS, formMS, links, reconn})
			})
		}
	}

	// build the whole table and print it in a single write, after all subtests, so the
	// testing framework's "=== RUN" / "--- PASS" lines cannot tear it apart.
	var b strings.Builder
	fmt.Fprintf(&b, "\n%-6s %-6s %-10s %-10s %-12s %-10s\n", "N", "pool", "conn(ms)", "form(ms)", "links", "reconnects")
	for _, r := range rows {
		fmt.Fprintf(&b, "%-6d %-6d %-10.1f %-10.1f %-12d %-10d\n", r.n, r.pool, r.connMS, r.formMS, r.links, r.reconn)
	}
	fmt.Print(b.String())
}

// benchOne forms a full mesh of n nodes with the given pool size and returns, all from
// the first dial: connMS (every ordered pair connected, its primary TCP up), formMS
// (every ordered pair holding exactly `pool` TCP links), the total reconnection count
// (0 means no churn), and the total TCP-link count across all connections (expect
// n*(n-1)*pool when fully formed, counting each connection from both ends).
func benchOne(t *testing.T, n, pool int) (connMS, formMS float64, reconn uint64, links int) {
	s := stage.New(t)
	nodes := make([]*stage.Node, n)
	for i := range nodes {
		nodes[i] = s.StartNode(fmt.Sprintf("b%03d", i), stage.NodeOptions{PoolSize: pool})
	}

	start := time.Now()
	var wg sync.WaitGroup
	for i := range nodes {
		for j := range nodes {
			if i == j {
				continue
			}
			wg.Add(1)
			go func(src, dst *stage.Node) {
				defer wg.Done()
				src.Native().Network().GetNode(dst.Name())
			}(nodes[i], nodes[j])
		}
	}
	wg.Wait()

	connSet := false
	deadline := time.Now().Add(120 * time.Second)
	for {
		missing := 0   // pairs with no connection yet (primary not up)
		unsettled := 0 // pairs not connected, or connected but pool not yet full
		for i := range nodes {
			for j := range nodes {
				if i == j {
					continue
				}
				r, err := nodes[i].Native().Network().Node(nodes[j].Name())
				if err != nil {
					missing++
					unsettled++
					nodes[i].Native().Network().GetNode(nodes[j].Name())
					continue
				}
				// TCP-link-count check: a connection is settled only when it holds
				// exactly its configured pool size (PoolLen == PoolSize), catching
				// undershoot and overshoot. When an explicit pool was requested
				// (pool > 0) also verify it propagated (PoolSize == pool); pool <= 0
				// means "use the framework default" (handshake substitutes it to 3), so
				// there is no requested size to check against.
				if info := r.Info(); info.PoolLen != info.PoolSize || (pool > 0 && info.PoolSize != pool) {
					unsettled++
				}
			}
		}
		// first moment every pair is connected (one link per pair), before pool fill
		if connSet == false && missing == 0 {
			connMS = float64(time.Since(start).Microseconds()) / 1000.0
			connSet = true
		}
		if unsettled == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("N=%d pool=%d did not form in 120s: %d unsettled", n, pool, unsettled)
		}
		time.Sleep(10 * time.Millisecond)
	}
	formMS = float64(time.Since(start).Microseconds()) / 1000.0

	for i := range nodes {
		for j := range nodes {
			if i == j {
				continue
			}
			if r, err := nodes[i].Native().Network().Node(nodes[j].Name()); err == nil {
				info := r.Info()
				reconn += info.Reconnections
				links += info.PoolLen
			}
		}
	}
	return connMS, formMS, reconn, links
}
