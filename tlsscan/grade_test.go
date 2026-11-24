package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

var agoraFixa = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

// certAutoAssinado gera em memória um cert auto-assinado com a validade
// pedida, para os testes não dependerem de arquivo nem de rede.
func certAutoAssinado(t *testing.T, notBefore, notAfter time.Time) *x509.Certificate {
	t.Helper()
	chave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gerando chave: %v", err)
	}
	modelo := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "teste.local"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		DNSNames:     []string{"teste.local"},
	}
	der, err := x509.CreateCertificate(rand.Reader, modelo, modelo, &chave.PublicKey, chave)
	if err != nil {
		t.Fatalf("criando cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("lendo cert: %v", err)
	}
	return cert
}

func achar(issues []issue, check string) (issue, bool) {
	for _, i := range issues {
		if i.Check == check {
			return i, true
		}
	}
	return issue{}, false
}

func TestAvaliarCertExpiradoEAutoAssinado(t *testing.T) {
	cert := certAutoAssinado(t, agoraFixa.AddDate(-2, 0, 0), agoraFixa.AddDate(-1, 0, 0))
	issues := avaliarCert(cert, agoraFixa)

	if i, ok := achar(issues, "cert-expirado"); !ok || i.Severity != sevCritical {
		t.Errorf("cert vencido deveria ser critical: %+v", issues)
	}
	if i, ok := achar(issues, "auto-assinado"); !ok || i.Severity != sevMedium {
		t.Errorf("cert auto-assinado deveria ser medium: %+v", issues)
	}
}

func TestAvaliarCertValidoNaoSinalizaExpiracao(t *testing.T) {
	cert := certAutoAssinado(t, agoraFixa.AddDate(0, -1, 0), agoraFixa.AddDate(1, 0, 0))
	issues := avaliarCert(cert, agoraFixa)

	for _, c := range []string{"cert-expirado", "cert-expira-logo"} {
		if _, ok := achar(issues, c); ok {
			t.Errorf("cert com 1 ano de folga não deveria gerar %s", c)
		}
	}
}

func TestAvaliarCertExpiraLogo(t *testing.T) {
	casos := []struct {
		dias int
		sev  string
	}{{10, sevHigh}, {20, sevMedium}}
	for _, c := range casos {
		cert := certAutoAssinado(t, agoraFixa.AddDate(0, -1, 0), agoraFixa.AddDate(0, 0, c.dias))
		i, ok := achar(avaliarCert(cert, agoraFixa), "cert-expira-logo")
		if !ok || i.Severity != c.sev {
			t.Errorf("expira em %d dias: quero %s, veio %+v", c.dias, c.sev, i)
		}
	}
}

func TestAvaliarVersoesTLS10EhHigh(t *testing.T) {
	issues := avaliarVersoes([]uint16{tls.VersionTLS10, tls.VersionTLS12})
	if len(issues) != 1 || issues[0].Severity != sevHigh {
		t.Fatalf("TLS 1.0 deveria gerar uma issue high: %+v", issues)
	}
}

func TestAvaliarVersoesModernasLimpas(t *testing.T) {
	if issues := avaliarVersoes([]uint16{tls.VersionTLS12, tls.VersionTLS13}); len(issues) != 0 {
		t.Errorf("TLS 1.2/1.3 não deveria gerar issue: %+v", issues)
	}
}

func TestAvaliarHSTS(t *testing.T) {
	if issues := avaliarHSTS(""); len(issues) != 1 || issues[0].Severity != sevMedium {
		t.Errorf("sem HSTS deveria ser medium: %+v", issues)
	}
	if issues := avaliarHSTS("max-age=31536000"); len(issues) != 0 {
		t.Errorf("com HSTS não deveria gerar issue: %+v", issues)
	}
}

func TestNota(t *testing.T) {
	casos := []struct {
		nome   string
		issues []issue
		quero  string
	}{
		{"sem issues", nil, "A"},
		{"uma medium", []issue{{Severity: sevMedium}}, "B"},
		{"duas medium", []issue{{Severity: sevMedium}, {Severity: sevMedium}}, "C"},
		{"uma high", []issue{{Severity: sevHigh}}, "D"},
		{"duas high", []issue{{Severity: sevHigh}, {Severity: sevHigh}}, "E"},
		{"critical vence tudo", []issue{{Severity: sevMedium}, {Severity: sevCritical}}, "F"},
	}
	for _, c := range casos {
		if got := nota(c.issues); got != c.quero {
			t.Errorf("%s: nota = %s, quero %s", c.nome, got, c.quero)
		}
	}
}
