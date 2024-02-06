package gen

import (
	"crypto/tls"
	"sync"
)

type CertManager interface {
	Update(cert tls.Certificate)
	GetCertificateFunc() func(*tls.ClientHelloInfo) (*tls.Certificate, error)
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
	return func(ch *tls.ClientHelloInfo) (*tls.Certificate, error) {
		cm.RLock()
		defer cm.RUnlock()
		return cm.cert, nil
	}
}
