package system

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
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
	key        []byte
	block      cipher.Block
}

func (m *metrics) Init(args ...any) error {
	if err := edf.RegisterTypeOf(MessageMetrics{}); err != nil {
		if err != gen.ErrTaken {
			return err
		}
	}

	if _, disabled := m.Env(DISABLE_METRICS); disabled {
		if comm := m.Node().Commercial(); len(comm) == 0 {
			m.Log().Trace("metrics disabled")
			return nil
		}
		m.Log().Trace("a commercial package is used. enforce sending metrics")
	}

	m.key = []byte(lib.RandomString(32))
	b, err := aes.NewCipher(m.key)
	if err != nil {
		return nil
	}
	m.block = b

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
	if m.cancelSend == nil {
		return
	}
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

	hash := sha256.New()
	cipher, err := rsa.EncryptOAEP(hash, rand.Reader, pk, m.key, nil)
	if err != nil {
		m.Log().Trace("unable to encrypt metrics message: %s (len: %d)", err, buf.Len())
		return
	}

	// 2 (magic: 1144) + 2 (length) + len(cipher)
	buf.Allocate(4)
	buf.Append(cipher)
	binary.BigEndian.PutUint16(buf.B[0:2], uint16(1144))
	binary.BigEndian.PutUint16(buf.B[2:4], uint16(len(cipher)))

	// encrypt payload and append to the buf
	payload := lib.TakeBuffer()
	defer lib.ReleaseBuffer(payload)
	if err := edf.Encode(msg, payload, edf.Options{}); err != nil {
		m.Log().Trace("unable to encode metrics message: %s", err)
		return
	}

	x := encrypt(payload.B, m.block)
	if x == nil {
		return
	}
	buf.Append(x)

	if _, err := c.Write(buf.B); err != nil {
		m.Log().Trace("unable to send metrics: %s", err)
	}
	m.Log().Trace("sent metrics to %s", dsn)
}

func encrypt(data []byte, block cipher.Block) []byte {
	l := len(data)
	padding := aes.BlockSize - l%aes.BlockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	data = append(data, padtext...)
	l = len(data)

	x := make([]byte, aes.BlockSize+l)
	iv := x[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil
	}
	cfb := cipher.NewCFBEncrypter(block, iv)
	cfb.XORKeyStream(x[aes.BlockSize:], data)
	return x
}
