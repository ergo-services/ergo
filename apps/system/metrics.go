package system

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"net"
	"runtime"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/lib/osdep"
	"github.com/ergo-services/ergo/node"
)

var (
	defaultMetricsPeriod = time.Minute
)

type systemMetrics struct {
	gen.Server
}

type systemMetricsState struct {
	// gather last 10 stats
	stats [10]nodeFullStats
	i     int
}
type messageSystemAnonInfo struct{}
type messageSystemGatherStats struct{}

type nodeFullStats struct {
	timestamp int64
	utime     int64
	stime     int64

	memAlloc      uint64
	memTotalAlloc uint64
	memFrees      uint64
	memSys        uint64
	memNumGC      uint32

	node    node.NodeStats
	network []node.NetworkStats
}

func (sb *systemMetrics) Init(process *gen.ServerProcess, args ...etf.Term) error {
	lib.Log("[%s] SYSTEM_METRICS: Init: %#v", process.NodeName(), args)
	if err := RegisterTypes(); err != nil {
		return err
	}
	options := args[0].(node.System)
	process.State = &systemMetricsState{}
	if options.DisableAnonMetrics == false {
		process.CastAfter(process.Self(), messageSystemAnonInfo{}, defaultMetricsPeriod)
	}
	process.CastAfter(process.Self(), messageSystemGatherStats{}, defaultMetricsPeriod)
	return nil
}

func (sb *systemMetrics) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	lib.Log("[%s] SYSTEM_METRICS: HandleCast: %#v", process.NodeName(), message)
	state := process.State.(*systemMetricsState)
	switch message.(type) {
	case messageSystemAnonInfo:
		ver := process.Env(node.EnvKeyVersion).(node.Version)
		sendAnonInfo(process.NodeName(), ver)

	case messageSystemGatherStats:
		stats := gatherStats(process)
		if state.i > len(state.stats)-1 {
			state.i = 0
		}
		state.stats[state.i] = stats
		state.i++
		process.CastAfter(process.Self(), messageSystemGatherStats{}, defaultMetricsPeriod)
	}
	return gen.ServerStatusOK
}

func (sb *systemMetrics) Terminate(process *gen.ServerProcess, reason string) {
	lib.Log("[%s] SYSTEM_METRICS: Terminate with reason %q", process.NodeName(), reason)
}

// private routines

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

	// FIXME get it back before the release
	// nameHash := crc32.Checksum([]byte(name), lib.CRC32Q)
	nameHash := name

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	message := MessageSystemAnonMetrics{
		Name:        nameHash,
		Arch:        runtime.GOARCH,
		OS:          runtime.GOOS,
		NumCPU:      runtime.NumCPU(),
		GoVersion:   runtime.Version(),
		ErgoVersion: ver.Release,
	}
	if err := etf.Encode(message, b, etf.EncodeOptions{}); err != nil {
		return
	}

	hash := sha256.New()
	cipher, err := rsa.EncryptOAEP(hash, rand.Reader, pk, b.B, nil)
	if err != nil {
		return
	}

	// 2 (magic: 1144) + 2 (length) + len(cipher)
	b.Reset()
	b.Allocate(4)
	b.Append(cipher)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(1144))
	binary.BigEndian.PutUint16(b.B[2:4], uint16(len(cipher)))
	c.Write(b.B)
}

func gatherStats(process *gen.ServerProcess) nodeFullStats {
	fullStats := nodeFullStats{}

	// CPU (windows doesn't support this feature)
	fullStats.utime, fullStats.stime = osdep.ResourceUsage()

	// Memory
	mem := runtime.MemStats{}
	runtime.ReadMemStats(&mem)
	fullStats.memAlloc = mem.Alloc
	fullStats.memTotalAlloc = mem.TotalAlloc
	fullStats.memSys = mem.Sys
	fullStats.memFrees = mem.Frees
	fullStats.memNumGC = mem.NumGC

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
	fullStats.timestamp = time.Now().Unix()
	return fullStats
}
