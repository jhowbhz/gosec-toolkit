package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// hostPorta separa "host:porta"; sem porta (ou porta ilegível) usa o padrão.
func hostPorta(h string, portaPadrao int) (string, int) {
	h = strings.TrimSpace(h)
	host, porta, err := net.SplitHostPort(h)
	if err != nil {
		return strings.Trim(h, "[]"), portaPadrao
	}
	n, err := strconv.Atoi(porta)
	if err != nil || n < 1 || n > 65535 {
		return host, portaPadrao
	}
	return host, n
}

// sansAoVivo faz o handshake TLS e devolve DNSNames + CN do certificado
// folha. Com --insecure o handshake não valida a cadeia: o objetivo é LER o
// cert, e um cert vencido ou autoassinado continua tendo nomes úteis. Sem
// --insecure, um cert inválido derruba o handshake e nada é coletado.
func sansAoVivo(ctx context.Context, host string, porta int, timeout time.Duration, insecure bool) ([]string, error) {
	cfg := &tls.Config{InsecureSkipVerify: insecure}
	if net.ParseIP(host) == nil {
		cfg.ServerName = host // SNI: sem ele muitos hosts devolvem o cert padrão
	}
	d := tls.Dialer{NetDialer: &net.Dialer{Timeout: timeout}, Config: cfg}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(porta)))
	if err != nil {
		return nil, fmt.Errorf("handshake %s:%d: %w", host, porta, err)
	}
	defer conn.Close()

	tc, ok := conn.(*tls.Conn)
	if !ok {
		return nil, errors.New("conexão não é TLS")
	}
	certs := tc.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("%s:%d não apresentou certificado", host, porta)
	}
	return nomesDoCert(certs[0].DNSNames, certs[0].Subject.CommonName), nil
}

// nomesDoCert junta SANs e CN. O CN só entra se parecer hostname: em certs
// antigos ele traz texto livre ("Fake LE Intermediate", nome de empresa).
func nomesDoCert(dnsNames []string, cn string) []string {
	nomes := append([]string(nil), dnsNames...)
	if n := normalizarNome(cn); strings.Contains(n, ".") && nomeValido(n) {
		nomes = append(nomes, n)
	}
	return nomes
}
