package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"time"
)

// certValidade é por quanto tempo o cert auto-assinado vale. Curto de
// propósito: é um cert de interceptação, não um cert de produção.
const certValidade = 365 * 24 * time.Hour

// selfSignedCert gera na hora um certificado auto-assinado que serve para
// o host interceptado (--from). O navegador vai avisar que não confia —
// isso é esperado: você confia manualmente no cert da sua própria máquina.
// Aceita tanto nome DNS quanto IP literal.
func selfSignedCert(host string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}

	now := time.Now()
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: host, Organization: []string{"proxyreverse (teste)"}},
		NotBefore:             now.Add(-time.Hour), // folga pra relógio dessincronizado
		NotAfter:              now.Add(certValidade),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	if ip := net.ParseIP(host); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{host}
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
		Leaf:        &tmpl,
	}, nil
}

// loadCert decide de onde vem o cert da escuta HTTPS: arquivo próprio
// (--cert/--key) quando informado, senão um auto-assinado pro host.
func loadCert(c Config) (tls.Certificate, error) {
	if c.CertFile != "" {
		return tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	}
	return selfSignedCert(c.fromHost())
}
