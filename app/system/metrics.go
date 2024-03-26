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
	"strconv"
	"strings"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/edf"
)

const (
	period time.Duration = time.Second * 300

	DISABLE_METRICS gen.Env = "disable_metrics"
	TRACE_METRICS   gen.Env = "trace_metrics"
)

type MessageMetrics struct {
	Name        gen.Atom
	Creation    int64
	Uptime      int64
	Arch        string
	OS          string
	NumCPU      int
	GoVersion   string
	Version     string
	ErgoVersion string
	Commercial  string
}

func factory_metrics() gen.ProcessBehavior {
	return &metrics{}
}

type doSendMetrics struct{}

type metrics struct {
	act.Actor
	cancelSend gen.CancelFunc
}

func (m *metrics) Init(args ...any) error {
	if err := edf.RegisterTypeOf(MessageMetrics{}); err != nil {
		if err != gen.ErrTaken {
			return err
		}
	}

	if _, disable := m.Env(DISABLE_METRICS); disable {
		m.Log().Trace("metrics disabled")
		return nil
	}

	m.Log().Trace("scheduled sending metrics in %v", period)
	m.cancelSend, _ = m.SendAfter(m.PID(), doSendMetrics{}, period)
	return nil
}

func (m *metrics) HandleMessage(from gen.PID, message any) error {

	switch message.(type) {
	case doSendMetrics:
		m.send()
		m.Log().Trace("scheduled sending metrics in %v", period)
		m.cancelSend, _ = m.SendAfter(m.PID(), doSendMetrics{}, period)

	default:
		m.Log().Trace("received unknown message: %#v", message)
	}
	return nil
}

func (m *metrics) Terminate(reason error) {
	m.cancelSend()
}

func (m *metrics) send() {
	var msrv = "metrics.ergo.services"

	values, err := net.LookupTXT(msrv)
	if err != nil || len(values) == 0 {
		m.Log().Trace("lookup TXT record in %s failed or returned empty result", msrv)
		return
	}
	v, err := base64.StdEncoding.DecodeString(values[0])
	if err != nil {
		return
	}

	pk, err := x509.ParsePKCS1PublicKey([]byte(v))
	if err != nil {
		m.Log().Trace("unable to parse public key (TXT record in %s)", msrv)
		return
	}

	_, srv, err := net.LookupSRV("data", "mt1", msrv)
	if err != nil || len(srv) == 0 {
		m.Log().Trace("unable to resolve SRV record: %s", err)
		return
	}

	dsn := net.JoinHostPort(strings.TrimSuffix(srv[0].Target, "."),
		strconv.Itoa(int(srv[0].Port)))
	c, err := net.Dial("udp", dsn)
	if err != nil {
		m.Log().Trace("unable to dial the host %s: %s", dsn, err)
		return
	}
	defer c.Close()

	msg := MessageMetrics{
		Name:        m.Node().Name(),
		Creation:    m.Node().Creation(),
		Uptime:      m.Node().Uptime(),
		Arch:        runtime.GOARCH,
		OS:          runtime.GOOS,
		NumCPU:      runtime.NumCPU(),
		GoVersion:   runtime.Version(),
		Version:     m.Node().Version().String(),
		ErgoVersion: m.Node().FrameworkVersion().String(),
		Commercial:  fmt.Sprintf("%v", m.Node().Commercial()),
	}

	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	if err := edf.Encode(msg, buf, edf.Options{}); err != nil {
		m.Log().Trace("unable to encode metrics message: %s", err)
		return
	}

	hash := sha256.New()
	cipher, err := rsa.EncryptOAEP(hash, rand.Reader, pk, buf.B, nil)
	if err != nil {
		m.Log().Trace("unable to encrypt metrics message: %s (len: %d)", err, buf.Len())
		return
	}

	// 2 (magic: 1144) + 2 (length) + len(cipher)
	buf.Reset()
	buf.Allocate(4)
	buf.Append(cipher)
	binary.BigEndian.PutUint16(buf.B[0:2], uint16(1144))
	binary.BigEndian.PutUint16(buf.B[2:4], uint16(len(cipher)))
	if _, err := c.Write(buf.B); err != nil {
		m.Log().Trace("unable to send metrics: %s", err)
	}
	m.Log().Trace("sent metrics to %s", dsn)
}
