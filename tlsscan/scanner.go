package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

// versoesTLS é a ordem do relatório. SSLv3 fica de fora: o crypto/tls do Go
// não o implementa, então nunca teríamos como aceitá-lo ou recusá-lo.
var versoesTLS = []uint16{tls.VersionTLS10, tls.VersionTLS11, tls.VersionTLS12, tls.VersionTLS13}

// escaneador junta a config e o relógio (injetável) para escanear alvos.
type escaneador struct {
	cfg   Config
	agora func() time.Time
}

// separarAlvo divide "host[:porta]" aplicando a porta padrão quando faltar.
func separarAlvo(alvo string, portaPadrao int) (host, porta string) {
	if h, p, err := net.SplitHostPort(alvo); err == nil {
		return h, p
	}
	return alvo, strconv.Itoa(portaPadrao)
}

// escanear faz o trabalho de um alvo. Falha de rede vira Erro no resultado
// (não error) porque um host fora do ar não pode interromper a lista.
func (e escaneador) escanear(alvo string) resultadoHost {
	host, porta := separarAlvo(alvo, e.cfg.Port)
	addr := net.JoinHostPort(host, porta)
	sni := e.cfg.ServerName
	if sni == "" {
		sni = host
	}

	res := resultadoHost{
		Timestamp: e.agora().Format(time.RFC3339),
		Host:      addr,
		Versoes:   []string{},
		Ciphers:   map[string]string{},
		Certs:     []certInfo{},
		Issues:    []issue{},
	}
	timeout := time.Duration(e.cfg.TimeoutS) * time.Second

	var aceitas []uint16
	var pares []*x509.Certificate
	for _, v := range versoesTLS {
		estado, err := handshake(addr, sni, v, timeout)
		if err != nil {
			continue
		}
		aceitas = append(aceitas, v)
		nome := tls.VersionName(v)
		res.Versoes = append(res.Versoes, nome)
		res.Ciphers[nome] = tls.CipherSuiteName(estado.CipherSuite)
		pares = estado.PeerCertificates // a última (mais nova) versão aceita vence
	}
	if len(aceitas) == 0 {
		res.Erro = "nenhuma versão TLS 1.0-1.3 completou o handshake"
		res.Grade = "-"
		return res
	}

	res.Issues = append(res.Issues, avaliarVersoes(aceitas)...)
	e.coletarCadeia(&res, pares, sni)
	if !e.cfg.NoHSTS {
		e.coletarHSTS(&res, addr, sni, timeout)
	}
	res.Grade = nota(res.Issues)
	return res
}

// handshake fixa min=max=versão. InsecureSkipVerify é intencional: aqui a
// pergunta é "o servidor aceita esta versão?", e um cert ruim não pode
// esconder a resposta; a verificação é feita à parte, em coletarCadeia.
func handshake(addr, sni string, versao uint16, timeout time.Duration) (tls.ConnectionState, error) {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", addr, &tls.Config{
		MinVersion:         versao,
		MaxVersion:         versao,
		InsecureSkipVerify: true,
		ServerName:         sni,
	})
	if err != nil {
		return tls.ConnectionState{}, err
	}
	defer conn.Close()
	return conn.ConnectionState(), nil
}

// coletarCadeia descreve os certs, avalia cada um e registra se a cadeia
// seria confiável para o SNI. O erro vai em campo próprio, não em issue,
// porque uma raiz interna ausente NESTA máquina não é fraqueza do alvo.
func (e escaneador) coletarCadeia(res *resultadoHost, pares []*x509.Certificate, sni string) {
	agora := e.agora()
	for i, c := range pares {
		if i > 0 && !e.cfg.FullChain {
			break
		}
		res.Certs = append(res.Certs, descreverCert(c, agora))
		res.Issues = append(res.Issues, avaliarCert(c, agora)...)
	}
	if len(pares) == 0 {
		return
	}
	inter := x509.NewCertPool()
	for _, c := range pares[1:] {
		inter.AddCert(c)
	}
	_, err := pares[0].Verify(x509.VerifyOptions{DNSName: sni, Intermediates: inter, CurrentTime: agora})
	if err != nil {
		res.VerificacaoEr = err.Error()
		return
	}
	res.VerificacaoOK = true
}

// coletarHSTS faz um GET sem seguir redirect: o header que interessa é o da
// resposta do próprio alvo, e o HSTS só vale quando vem de https.
func (e escaneador) coletarHSTS(res *resultadoHost, addr, sni string, timeout time.Duration) {
	cli := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true, ServerName: sni},
			DisableKeepAlives: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequest(http.MethodGet, "https://"+addr+"/", nil)
	if err != nil {
		res.HSTSErro = err.Error()
		return
	}
	req.Host = sni
	req.Header.Set("User-Agent", "tlsscan/1.0 (+https://github.com/jhowbhz/tlsscan)")
	resp, err := cli.Do(req)
	if err != nil {
		res.HSTSErro = err.Error()
		return
	}
	resp.Body.Close()
	res.HSTS = resp.Header.Get("Strict-Transport-Security")
	res.Issues = append(res.Issues, avaliarHSTS(res.HSTS)...)
}

func descreverCert(c *x509.Certificate, agora time.Time) certInfo {
	return certInfo{
		Subject:      c.Subject.CommonName,
		SANs:         c.DNSNames,
		Emissor:      c.Issuer.CommonName,
		NotBefore:    c.NotBefore.Format("2006-01-02"),
		NotAfter:     c.NotAfter.Format("2006-01-02"),
		DiasParaExp:  int(c.NotAfter.Sub(agora).Hours() / 24),
		Assinatura:   c.SignatureAlgorithm.String(),
		Chave:        descreverChave(c.PublicKey),
		AutoAssinado: autoAssinado(c),
	}
}

func descreverChave(pub any) string {
	switch k := pub.(type) {
	case *rsa.PublicKey:
		return fmt.Sprintf("RSA %d", k.N.BitLen())
	case *ecdsa.PublicKey:
		return fmt.Sprintf("ECDSA %s", k.Curve.Params().Name)
	case ed25519.PublicKey:
		return "Ed25519"
	}
	return fmt.Sprintf("%T", pub)
}
