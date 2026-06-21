package distributed

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"ergo.services/ergo/testing/stage"
)

// TestMeshBench measures full-mesh formation time and reconnection churn across a
// grid of cluster sizes and pool depths. Gated behind ERGO_MESH_BENCH so it never
// runs in the normal suite. Run one config per fresh process to avoid TIME_WAIT
// accumulation skewing the larger configs:
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

	fmt.Printf("%-6s %-6s %-12s %-10s\n", "N", "pool", "form(ms)", "reconnects")
	for _, n := range Ns {
		for _, pool := range Pools {
			t.Run(fmt.Sprintf("N%d_pool%d", n, pool), func(t *testing.T) {
				ms, reconn := benchOne(t, n, pool)
				fmt.Printf("%-6d %-6d %-12.1f %-10d\n", n, pool, ms, reconn)
			})
		}
	}
}

// benchOne forms a full mesh of n nodes with the given pool size and returns the
// formation time (ms, from first dial to every ordered pair having a filled pool)
// and the total reconnection count across all connections.
func benchOne(t *testing.T, n, pool int) (float64, uint64) {
	s := stage.New(t)
	nodes := make([]*stage.Node, n)
	for i := range nodes {
		nodes[i] = s.Node(fmt.Sprintf("b%03d", i), stage.NodeOptions{PoolSize: pool})
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

	deadline := time.Now().Add(120 * time.Second)
	for {
		unsettled := 0
		for i := range nodes {
			for j := range nodes {
				if i == j {
					continue
				}
				r, err := nodes[i].Native().Network().Node(nodes[j].Name())
				if err != nil {
					unsettled++
					nodes[i].Native().Network().GetNode(nodes[j].Name())
					continue
				}
				if info := r.Info(); info.PoolLen != info.PoolSize {
					unsettled++
				}
			}
		}
		if unsettled == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("N=%d pool=%d did not form in 120s: %d unsettled", n, pool, unsettled)
		}
		time.Sleep(10 * time.Millisecond)
	}
	formMS := float64(time.Since(start).Microseconds()) / 1000.0

	var reconn uint64
	for i := range nodes {
		for j := range nodes {
			if i == j {
				continue
			}
			if r, err := nodes[i].Native().Network().Node(nodes[j].Name()); err == nil {
				reconn += r.Info().Reconnections
			}
		}
	}
	return formMS, reconn
}
