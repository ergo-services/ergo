package system

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net"
	"runtime"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/lib/osdep"
	"github.com/ergo-services/ergo/node"
)

type systemMetrics struct {
	gen.Server
}

type systemMetricsState struct{}
type messageSystemAnonInfo struct{}
type messageSystemGatherStat struct{}

type nodeFullStats struct {
	utime    int64
	stime    int64
	maxProcs int
	cpus     int

	memAlloc uint64
	memFrees uint64
	memSys   uint64

	node    node.NodeStats
	network []node.NetworkStats
}

func (sb *systemMetrics) Init(process *gen.ServerProcess, args ...etf.Term) error {
	lib.Log("SYSTEM_METRICS: Init: %#v", args)

	process.State = &systemMetricsState{}
	process.CastAfter(process.Self(), messageSystemAnonInfo{}, 10*time.Second)
	return nil
}

func (sb *systemMetrics) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	lib.Log("SYSTEM_METRICS: HandleCast: %#v", message)
	switch message.(type) {
	case messageSystemAnonInfo:
		fmt.Println("ANON")
		ver := process.Env(node.EnvKeyVersion).(node.Version)
		sendAnonInfo(process.NodeName(), ver)
		process.CastAfter(process.Self(), messageSystemGatherStat{}, 10*time.Second)
	case messageSystemGatherStat:
		fmt.Println("GATHER")
		gatherStat(process)
		process.CastAfter(process.Self(), messageSystemGatherStat{}, 10*time.Second)
	}
	return gen.ServerStatusOK
}

func (sb *systemMetrics) Terminate(process *gen.ServerProcess, reason string) {
	lib.Log("SYSTEM_METRICS: Terminate with reason %q", reason)
}

func sendAnonInfo(name string, ver node.Version) {
	metricsHost := "metrics.ergo.services"

	values, err := net.LookupTXT(metricsHost)
	if err != nil || len(values) == 0 {
		return
	}

	v, err := base64.StdEncoding.DecodeString(values[0])
	if err != nil {
		return
	}

	pk, err := x509.ParsePKCS1PublicKey([]byte(v))
	if err != nil {
		return
	}

	c, err := net.Dial("udp", metricsHost+":4411")
	if err != nil {
		return
	}
	defer c.Close()

	data := fmt.Sprintf("1|%s|%s|%s|%d|%s|%s", name, runtime.GOARCH, runtime.GOOS,
		runtime.NumCPU(), runtime.Version(), ver.Release)

	hash := sha256.New()
	cipher, err := rsa.EncryptOAEP(hash, rand.Reader, pk, []byte(data), nil)
	if err != nil {
		return
	}

	// 2 (magic: 4411) + 2 (length) + len(cipher)
	buf := make([]byte, 2+2+len(cipher))
	binary.BigEndian.PutUint16(buf[0:2], uint16(4411))
	binary.BigEndian.PutUint16(buf[2:4], uint16(len(cipher)))
	copy(buf[4:], cipher)

	c.Write(buf)
}

func gatherStat(process *gen.ServerProcess) nodeFullStats {
	fullStats := nodeFullStats{}

	// CPU (windows doesn't support this feature)
	fullStats.utime, fullStats.stime = osdep.ResourceUsage()
	fullStats.maxProcs = runtime.GOMAXPROCS(0)
	fullStats.cpus = runtime.NumCPU()

	// Memory
	mem := runtime.MemStats{}
	runtime.ReadMemStats(&mem)
	fullStats.memAlloc = mem.HeapAlloc
	fullStats.memSys = mem.Sys
	fullStats.memFrees = mem.Frees

	// Network
	node := process.Env(node.EnvKeyNode).(node.Node)
	for _, name := range node.Nodes() {
		ns, err := node.NetworkStats(name)
		if err != nil {
			continue
		}
		fullStats.network = append(fullStats.network, ns)
	}

	fullStats.node = node.Stats()
	//fmt.Printf("STATS %#v \n", fullStats)
	return fullStats
}
