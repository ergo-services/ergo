package gen

import (
	"crypto/tls"
	"crypto/x509"
	"sync"
)

type CertManager interface {
	Update(cert tls.Certificate)
	GetCertificateFunc() func(*tls.ClientHelloInfo) (*tls.Certificate, error)
	GetCertificate() tls.Certificate
}

// CertAuthManager extends CertManager with CA pool management for mTLS.
// Provides runtime updates for both certificates and CA pools.
type CertAuthManager interface {
	CertManager

	// server-side: CA pool to verify client certificates
	SetClientCAs(pool *x509.CertPool)
	ClientCAs() *x509.CertPool

	// client-side: CA pool to verify server certificates
	SetRootCAs(pool *x509.CertPool)
	RootCAs() *x509.CertPool

	// server-side: client authentication policy
	SetClientAuth(auth tls.ClientAuthType)
	ClientAuth() tls.ClientAuthType

	// client-side: server name for SNI and verification
	SetServerName(name string)
	ServerName() string
}

type certManager struct {
	sync.RWMutex
	cert *tls.Certificate
}

func CreateCertManager(cert tls.Certificate) CertManager {
	return &certManager{
		cert: &cert,
	}
}

func (cm *certManager) Update(cert tls.Certificate) {
	cm.Lock()
	defer cm.Unlock()
	cm.cert = &cert
}

func (cm *certManager) GetCertificateFunc() func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		cm.RLock()
		defer cm.RUnlock()
		return cm.cert, nil
	}
}

func (cm *certManager) GetCertificate() tls.Certificate {
	cm.RLock()
	defer cm.RUnlock()
	return *cm.cert
}

type certAuthManager struct {
	certManager

	clientCAs  *x509.CertPool
	rootCAs    *x509.CertPool
	clientAuth tls.ClientAuthType
	serverName string
}

// CreateCertAuthManager creates a CertAuthManager with the given certificate.
// Use Set* methods to configure CA pools and authentication settings.
func CreateCertAuthManager(cert tls.Certificate) CertAuthManager {
	return &certAuthManager{
		certManager: certManager{cert: &cert},
		clientAuth:  tls.NoClientCert,
	}
}

func (m *certAuthManager) SetClientCAs(pool *x509.CertPool) {
	m.Lock()
	m.clientCAs = pool
	m.Unlock()
}

func (m *certAuthManager) ClientCAs() *x509.CertPool {
	m.RLock()
	defer m.RUnlock()
	return m.clientCAs
}

func (m *certAuthManager) SetRootCAs(pool *x509.CertPool) {
	m.Lock()
	m.rootCAs = pool
	m.Unlock()
}

func (m *certAuthManager) RootCAs() *x509.CertPool {
	m.RLock()
	defer m.RUnlock()
	return m.rootCAs
}

func (m *certAuthManager) SetClientAuth(auth tls.ClientAuthType) {
	m.Lock()
	m.clientAuth = auth
	m.Unlock()
}

func (m *certAuthManager) ClientAuth() tls.ClientAuthType {
	m.RLock()
	defer m.RUnlock()
	return m.clientAuth
}

func (m *certAuthManager) SetServerName(name string) {
	m.Lock()
	m.serverName = name
	m.Unlock()
}

func (m *certAuthManager) ServerName() string {
	m.RLock()
	defer m.RUnlock()
	return m.serverName
}
