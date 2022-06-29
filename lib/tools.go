package lib

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"math"
	"math/big"
	"sync"
	"time"
)

// Buffer
type Buffer struct {
	B        []byte
	original []byte
}

var (
	ergoTrace     = false
	ergoWarning   = false
	ergoNoRecover = false

	DefaultBufferLength = 16384
	buffers             = &sync.Pool{
		New: func() interface{} {
			b := &Buffer{
				B: make([]byte, 0, DefaultBufferLength),
			}
			b.original = b.B
			return b
		},
	}

	timers = &sync.Pool{
		New: func() interface{} {
			return time.NewTimer(time.Second * 5)
		},
	}

	ErrTooLarge = fmt.Errorf("Too large")

	CRC32Q = crc32.MakeTable(0xD5828281)
)

func init() {
	flag.BoolVar(&ergoTrace, "ergo.trace", false, "enable/disable extended debug info")
	flag.BoolVar(&ergoWarning, "ergo.warning", true, "enable/disable warning messages")
	flag.BoolVar(&ergoNoRecover, "ergo.norecover", false, "disable panic catching")
}

// Log
func Log(f string, a ...interface{}) {
	if ergoTrace {
		log.Printf(f, a...)
	}
}

// Warning
func Warning(f string, a ...interface{}) {
	if ergoWarning {
		log.Printf("WARNING! "+f, a...)
	}
}

// CatchPanic
func CatchPanic() bool {
	return ergoNoRecover == false
}

// TakeTimer
func TakeTimer() *time.Timer {
	return timers.Get().(*time.Timer)
}

// ReleaseTimer
func ReleaseTimer(t *time.Timer) {
	t.Stop()
	timers.Put(t)
}

// TakeBuffer
func TakeBuffer() *Buffer {
	return buffers.Get().(*Buffer)
}

// ReleaseBuffer
func ReleaseBuffer(b *Buffer) {
	c := cap(b.B)
	// cO := cap(b.original)
	// overlaps := c > 0 && cO > 0 && &(x[:c][c-1]) == &(y[:cO][cO-1])
	if c > DefaultBufferLength && c < 65536 {
		// reallocation happened. keep reallocated buffer as an original
		// if it doesn't exceed the size of 65K (we don't want to keep
		// too big slices)
		b.original = b.B[:0]
	}
	b.B = b.original[:0]
	buffers.Put(b)
}

// Reset
func (b *Buffer) Reset() {
	c := cap(b.B)
	if c > DefaultBufferLength && c < 65536 {
		b.original = b.B[:0]
	}
	// use the original start point of the slice
	b.B = b.original[:0]
}

// Set
func (b *Buffer) Set(v []byte) {
	b.B = append(b.B[:0], v...)
}

// AppendByte
func (b *Buffer) AppendByte(v byte) {
	b.B = append(b.B, v)
}

// Append
func (b *Buffer) Append(v []byte) {
	b.B = append(b.B, v...)
}

// String
func (b *Buffer) String() string {
	return string(b.B)
}

// Len
func (b *Buffer) Len() int {
	return len(b.B)
}

// WriteDataTo
func (b *Buffer) WriteDataTo(w io.Writer) error {
	l := len(b.B)
	if l == 0 {
		return nil
	}

	for {
		n, e := w.Write(b.B)
		if e != nil {
			return e
		}

		l -= n
		if l > 0 {
			continue
		}

		break
	}

	b.Reset()
	return nil
}

// ReadDataFrom
func (b *Buffer) ReadDataFrom(r io.Reader, limit int) (int, error) {
	capB := cap(b.B)
	lenB := len(b.B)
	if limit == 0 {
		limit = math.MaxInt
	}
	// if buffer becomes too large
	if lenB > limit {
		return 0, ErrTooLarge
	}
	if capB-lenB < capB>>1 {
		// less than (almost) 50% space left. increase capacity
		b.increase()
		capB = cap(b.B)
	}
	n, e := r.Read(b.B[lenB:capB])
	l := lenB + n
	b.B = b.B[:l]
	return n, e
}

func (b *Buffer) Write(v []byte) (n int, err error) {
	b.B = append(b.B, v...)
	return len(v), nil
}

func (b *Buffer) Read(v []byte) (n int, err error) {
	copy(v, b.B)
	return len(b.B), io.EOF
}

func (b *Buffer) increase() {
	cap1 := cap(b.B) * 8
	b1 := make([]byte, cap(b.B), cap1)
	copy(b1, b.B)
	b.B = b1
}

// Allocate
func (b *Buffer) Allocate(n int) {
	for {
		if cap(b.B) < n {
			b.increase()
			continue
		}
		b.B = b.B[:n]
		return
	}
}

// Extend
func (b *Buffer) Extend(n int) []byte {
	l := len(b.B)
	e := l + n
	for {
		if e > cap(b.B) {
			b.increase()
			continue
		}
		b.B = b.B[:e]
		return b.B[l:e]
	}
}

// RandomString
func RandomString(length int) string {
	buff := make([]byte, length/2)
	rand.Read(buff)
	return hex.EncodeToString(buff)
}

// GenerateSelfSignedCert
func GenerateSelfSignedCert(org string) (tls.Certificate, error) {
	var cert = tls.Certificate{}
	certPrivKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return cert, err
	}
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return cert, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{org},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24 * 365),
		IsCA:      true,

		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certBytes, err1 := x509.CreateCertificate(rand.Reader, &template, &template,
		&certPrivKey.PublicKey, certPrivKey)
	if err1 != nil {
		return cert, err1
	}

	certPEM := new(bytes.Buffer)
	pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	certPrivKeyPEM := new(bytes.Buffer)
	x509Encoded, _ := x509.MarshalECPrivateKey(certPrivKey)
	pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509Encoded,
	})

	return tls.X509KeyPair(certPEM.Bytes(), certPrivKeyPEM.Bytes())
}
