package main

import (
	"crypto/x509"
	"testing"
	"time"
)

func TestSelfSignedCertServeParaOHost(t *testing.T) {
	cert, err := selfSignedCert("meusite.com.br")
	if err != nil {
		t.Fatalf("selfSignedCert: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("certificado vazio")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("cert gerado não parseia: %v", err)
	}
	// O que dói se regredir: navegador não aceita o cert pro host interceptado.
	if err := leaf.VerifyHostname("meusite.com.br"); err != nil {
		t.Errorf("cert não serve pro host: %v", err)
	}
	now := time.Now()
	if now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
		t.Errorf("cert fora da validade: %s .. %s", leaf.NotBefore, leaf.NotAfter)
	}
}

func TestSelfSignedCertServeParaIP(t *testing.T) {
	cert, err := selfSignedCert("127.0.0.1")
	if err != nil {
		t.Fatalf("selfSignedCert: %v", err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("cert gerado não parseia: %v", err)
	}
	if err := leaf.VerifyHostname("127.0.0.1"); err != nil {
		t.Errorf("cert não serve pro IP: %v", err)
	}
}
