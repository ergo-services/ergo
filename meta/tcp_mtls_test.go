package meta

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

// genMTLSCert makes a self-signed cert usable as its own CA root and as both a server
// and a client certificate (ServerAuth + ClientAuth extended key usage), so one cert
// drives the whole mTLS exchange in the test.
func genMTLSCert(t *testing.T) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"ergo-mtls-test"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, leaf
}

// TestTCPServerMutualTLSDynamic: a meta TCP server given a CertAuthManager must enforce
// mTLS (RequireAndVerifyClientCert + ClientCAs), and a runtime SetClientCAs must take
// effect on the live listener.
func TestTCPServerMutualTLSDynamic(t *testing.T) {
	cert, leaf := genMTLSCert(t)

	cam := gen.CreateCertAuthManager(cert)
	cam.SetClientAuth(tls.RequireAndVerifyClientCert)
	cam.SetClientCAs(x509.NewCertPool()) // empty: no client cert trusted yet

	mb, err := CreateTCPServer(TCPServerOptions{Host: "127.0.0.1", Port: 0, CertManager: cam})
	if err != nil {
		t.Fatal(err)
	}
	s := mb.(*tcpserver)
	ln := s.listener
	t.Cleanup(func() { ln.Close() })
	addr := ln.Addr().String()

	// drive the server side of each handshake
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				c.(*tls.Conn).Handshake()
				c.Close()
			}(conn)
		}
	}()

	dial := func() error {
		c, err := tls.Dial("tcp", addr, &tls.Config{
			Certificates:       []tls.Certificate{cert}, // present the client cert
			InsecureSkipVerify: true,                    // not verifying the server here
			// TLS 1.2 so the client observes client-cert rejection during Dial (in 1.3 the
			// client handshake completes optimistically before the server verifies).
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS12,
		})
		if err == nil {
			c.Close()
		}
		return err
	}

	// empty CA pool -> the client cert is not trusted -> the handshake is rejected
	if err := dial(); err == nil {
		t.Fatal("handshake succeeded with an empty client CA pool: mTLS not enforced")
	}

	// add the CA at runtime -> the live listener must now accept the client cert
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	cam.SetClientCAs(pool)
	if err := dial(); err != nil {
		t.Fatalf("handshake failed after runtime SetClientCAs (update not applied to the live listener): %v", err)
	}
}
