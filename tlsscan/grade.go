package main

import (
	"bytes"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"
	"time"
)

// Severidades, da mais grave para a menos. Strings (e não int) porque vão
// direto para o JSONL, que é lido por gente.
const (
	sevCritical = "critical"
	sevHigh     = "high"
	sevMedium   = "medium"
)

// issue é uma fraqueza achada. Check é um identificador estável (dá para
// filtrar no JSONL); Detalhe é a frase para o humano.
type issue struct {
	Check    string `json:"check"`
	Severity string `json:"severity"`
	Detalhe  string `json:"detalhe"`
}

// Limiares de expiração e de chave. Ficam como constantes nomeadas porque são
// política (quanto antes avisar), não matemática.
const (
	diasAlto  = 15
	diasMedio = 30
	rsaMinimo = 2048
)

// avaliarCert aplica as regras de certificado. É pura (recebe "agora") para
// que o teste não dependa do relógio da máquina.
func avaliarCert(cert *x509.Certificate, agora time.Time) []issue {
	var out []issue

	if agora.After(cert.NotAfter) {
		out = append(out, issue{"cert-expirado", sevCritical,
			fmt.Sprintf("expirou em %s", cert.NotAfter.Format("2006-01-02"))})
	} else {
		dias := int(cert.NotAfter.Sub(agora).Hours() / 24)
		switch {
		case dias < diasAlto:
			out = append(out, issue{"cert-expira-logo", sevHigh, fmt.Sprintf("expira em %d dias", dias)})
		case dias < diasMedio:
			out = append(out, issue{"cert-expira-logo", sevMedium, fmt.Sprintf("expira em %d dias", dias)})
		}
	}
	if agora.Before(cert.NotBefore) {
		out = append(out, issue{"cert-ainda-invalido", sevHigh,
			fmt.Sprintf("só vale a partir de %s", cert.NotBefore.Format("2006-01-02"))})
	}

	if assinaturaFraca(cert.SignatureAlgorithm) {
		out = append(out, issue{"assinatura-fraca", sevHigh,
			fmt.Sprintf("algoritmo de assinatura %s", cert.SignatureAlgorithm)})
	}
	if pub, ok := cert.PublicKey.(*rsa.PublicKey); ok && pub.N.BitLen() < rsaMinimo {
		out = append(out, issue{"rsa-curta", sevHigh,
			fmt.Sprintf("chave RSA de %d bits (mínimo %d)", pub.N.BitLen(), rsaMinimo)})
	}
	if autoAssinado(cert) {
		out = append(out, issue{"auto-assinado", sevMedium,
			"certificado auto-assinado (se for intencional, ex.: ambiente interno, trate como info)"})
	}
	return out
}

// assinaturaFraca cobre MD2/MD5/SHA-1 por nome: a stdlib não expõe um
// predicado pronto e a lista de enums cresce a cada versão.
func assinaturaFraca(alg x509.SignatureAlgorithm) bool {
	nome := alg.String()
	return strings.Contains(nome, "SHA1") || strings.Contains(nome, "MD5") || strings.Contains(nome, "MD2")
}

// autoAssinado exige emissor == sujeito E assinatura que fecha com a própria
// chave; só comparar nomes marcaria como auto-assinada uma CA que reemite com
// o mesmo nome. Usa CheckSignature e não CheckSignatureFrom porque este
// exige IsCA/KeyUsageCertSign, que um certificado de servidor auto-assinado
// típico não tem.
func autoAssinado(cert *x509.Certificate) bool {
	return bytes.Equal(cert.RawIssuer, cert.RawSubject) &&
		cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature) == nil
}

// avaliarVersoes sinaliza protocolos obsoletos aceitos pelo servidor.
// SSLv3 nunca aparece: o crypto/tls do Go não o implementa.
func avaliarVersoes(aceitas []uint16) []issue {
	var out []issue
	for _, v := range aceitas {
		if v == tls.VersionTLS10 || v == tls.VersionTLS11 {
			out = append(out, issue{"protocolo-obsoleto", sevHigh,
				fmt.Sprintf("servidor aceita %s", tls.VersionName(v))})
		}
	}
	return out
}

// avaliarHSTS recebe o valor do header ("" = ausente) de uma resposta https.
func avaliarHSTS(header string) []issue {
	if strings.TrimSpace(header) == "" {
		return []issue{{"sem-hsts", sevMedium, "resposta https sem Strict-Transport-Security"}}
	}
	return nil
}

// nota resume as issues numa letra. Duas ou mais "high" valem E porque
// acumular fraquezas graves é pior que uma só, mas ainda não é "critical".
func nota(issues []issue) string {
	var altas, medias int
	for _, i := range issues {
		switch i.Severity {
		case sevCritical:
			return "F"
		case sevHigh:
			altas++
		case sevMedium:
			medias++
		}
	}
	switch {
	case altas >= 2:
		return "E"
	case altas == 1:
		return "D"
	case medias >= 2:
		return "C"
	case medias == 1:
		return "B"
	}
	return "A"
}
