package local

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"testing"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/gen"
)

// mtlsCert makes a self-signed cert usable as its own CA root and as both a server and a
// client certificate (ServerAuth + ClientAuth extended key usage).
func mtlsCert(t *testing.T) (tls.Certificate, *x509.Certificate) {
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

// freePort binds :0, grabs the assigned port and releases it for the node to reuse.
func freePort(t *testing.T) uint16 {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	ln.Close()
	return port
}

// TestLocalAcceptorMutualTLSDynamic: the node acceptor built from a CertAuthManager must
// enforce mTLS and pick up runtime CA-pool updates on the live listener (not freeze the
// pool at listener creation).
func TestLocalAcceptorMutualTLSDynamic(t *testing.T) {
	cert, leaf := mtlsCert(t)

	cam := gen.CreateCertAuthManager(cert)
	cam.SetClientAuth(tls.RequireAndVerifyClientCert)
	cam.SetClientCAs(x509.NewCertPool()) // empty: no client cert trusted yet

	port := freePort(t)
	opts := gen.NodeOptions{
		Network: gen.NetworkOptions{
			Cookie: "mtls",
			Acceptors: []gen.AcceptorOptions{
				{Host: "127.0.0.1", Port: port, PortRange: 1, CertManager: cam, InsecureSkipVerify: true},
			},
		},
	}
	opts.Log.DefaultLogger.Disable = true
	n, err := ergo.StartNode("mtls269@localhost", opts)
	if err != nil {
		t.Fatalf("StartNode: %v", err)
	}
	defer n.StopForce()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	dial := func() error {
		c, err := tls.Dial("tcp", addr, &tls.Config{
			Certificates:       []tls.Certificate{cert},
			InsecureSkipVerify: true,
			// TLS 1.2 so the client observes client-cert rejection during Dial.
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS12,
		})
		if err == nil {
			c.Close()
		}
		return err
	}

	// empty CA pool -> client cert not trusted -> handshake rejected
	if err := dial(); err == nil {
		t.Fatal("handshake succeeded with an empty client CA pool: mTLS not enforced on the acceptor")
	}

	// add the CA at runtime -> the live listener must now accept the client cert
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	cam.SetClientCAs(pool)
	if err := dial(); err != nil {
		t.Fatalf("handshake failed after runtime SetClientCAs (update not applied to the live acceptor): %v", err)
	}
}

// TestLocalAcceptorCertRotation: the acceptor built from a CertAuthManager must present the
// certificate that a runtime cam.Update installs, not one frozen at listener creation.
func TestLocalAcceptorCertRotation(t *testing.T) {
	certA, _ := mtlsCert(t)
	cam := gen.CreateCertAuthManager(certA) // server-only TLS (no client auth) is enough here

	port := freePort(t)
	opts := gen.NodeOptions{
		Network: gen.NetworkOptions{
			Cookie: "mtls",
			Acceptors: []gen.AcceptorOptions{
				{Host: "127.0.0.1", Port: port, PortRange: 1, CertManager: cam, InsecureSkipVerify: true},
			},
		},
	}
	opts.Log.DefaultLogger.Disable = true
	n, err := ergo.StartNode("mtlscert@localhost", opts)
	if err != nil {
		t.Fatalf("StartNode: %v", err)
	}
	defer n.StopForce()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	peerCert := func() []byte {
		c, err := tls.Dial("tcp", addr, &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
			MaxVersion:         tls.VersionTLS12,
		})
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer c.Close()
		return c.ConnectionState().PeerCertificates[0].Raw
	}

	if bytes.Equal(peerCert(), certA.Certificate[0]) == false {
		t.Fatal("acceptor did not present the initial certificate")
	}

	// rotate the certificate at runtime -> a new connection must see the new one
	certB, _ := mtlsCert(t)
	cam.Update(certB)
	if bytes.Equal(peerCert(), certB.Certificate[0]) == false {
		t.Fatal("acceptor did not present the rotated certificate (runtime Update ignored)")
	}
}
