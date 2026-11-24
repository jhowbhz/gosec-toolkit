package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"hash"
	"strings"
	"testing"
	"time"
)

// montarJWT gera um JWT HS* à mão (hmac + base64url), sem depender de lib:
// o teste precisa de um gabarito independente do código que verifica.
func montarJWT(t *testing.T, header, payload, segredo string, novo func() hash.Hash) string {
	t.Helper()
	enc := base64.RawURLEncoding.EncodeToString
	entrada := enc([]byte(header)) + "." + enc([]byte(payload))
	mac := hmac.New(novo, []byte(segredo))
	mac.Write([]byte(entrada))
	return entrada + "." + enc(mac.Sum(nil))
}

func jwtHS256(t *testing.T, payload, segredo string) string {
	return montarJWT(t, `{"alg":"HS256","typ":"JWT","kid":"k1"}`, payload, segredo, sha256.New)
}

// Guarda a promessa central: achar o segredo fraco certo e não dar falso
// positivo com o errado. Falso negativo aqui = o usuário acha que o segredo é forte.
func TestVerificarHMACAchaSegredoCertoERejeitaErrado(t *testing.T) {
	casos := []struct {
		alg  string
		novo func() hash.Hash
	}{
		{"HS256", sha256.New},
		{"HS384", sha512.New384},
		{"HS512", sha512.New},
	}
	for _, c := range casos {
		raw := montarJWT(t, `{"alg":"`+c.alg+`"}`, `{"sub":"ana"}`, "segredo123", c.novo)
		tok, err := decodificar(raw)
		if err != nil {
			t.Fatalf("%s: decodificar: %v", c.alg, err)
		}
		if !verificarHMAC(tok.signingInput, tok.assinatura, "segredo123", c.alg) {
			t.Errorf("%s: segredo correto deveria validar", c.alg)
		}
		if verificarHMAC(tok.signingInput, tok.assinatura, "errado", c.alg) {
			t.Errorf("%s: segredo errado não pode validar", c.alg)
		}
	}
}

func TestVerificarHMACIgnoraAlgNaoHMAC(t *testing.T) {
	if verificarHMAC("a.b", []byte{1, 2, 3}, "x", "RS256") {
		t.Error("RS256 não é verificável por segredo")
	}
	if verificarHMAC("a.b", nil, "x", "HS256") {
		t.Error("sem assinatura não há o que verificar")
	}
}

func TestTestarSegredosEncontraCandidato(t *testing.T) {
	tok, _ := decodificar(jwtHS256(t, `{"sub":"ana"}`, "admin"))

	achou, segredo := testarSegredos(tok, []string{"123456", "admin", "senha"})
	if !achou || segredo != "admin" {
		t.Errorf("achou=%v segredo=%q, quero true/admin", achou, segredo)
	}
	if achou, _ := testarSegredos(tok, []string{"123456", "senha"}); achou {
		t.Error("sem o candidato certo não deve achar nada")
	}
}

func TestDecodificarLeClaims(t *testing.T) {
	tok, err := decodificar(jwtHS256(t, `{"iss":"api","sub":"ana","exp":2000000000}`, "s"))
	if err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if tok.alg() != "HS256" || tok.kid() != "k1" {
		t.Errorf("alg=%q kid=%q, quero HS256/k1", tok.alg(), tok.kid())
	}
	if tok.Payload["iss"] != "api" || tok.Payload["sub"] != "ana" {
		t.Errorf("payload = %v", tok.Payload)
	}
	if exp, ok := claimTempo(tok.Payload, "exp"); !ok || exp.Unix() != 2000000000 {
		t.Errorf("exp = %v (ok=%v), quero 2000000000", exp, ok)
	}
}

func TestDecodificarRejeitaMalformado(t *testing.T) {
	for _, raw := range []string{"", "a.b", "a.b.c.d", "!!!.@@@.###"} {
		if _, err := decodificar(raw); err == nil {
			t.Errorf("decodificar(%q) deveria falhar", raw)
		}
	}
}

func TestDecodeSegmentToleraPadding(t *testing.T) {
	sem, err1 := decodeSegment("aGVsbG8")
	com, err2 := decodeSegment("aGVsbG8=")
	if err1 != nil || err2 != nil {
		t.Fatalf("erros: %v / %v", err1, err2)
	}
	if string(sem) != "hello" || string(com) != "hello" {
		t.Errorf("sem=%q com=%q, quero hello", sem, com)
	}
}

func TestAlgNoneSemAssinaturaESinalizado(t *testing.T) {
	enc := base64.RawURLEncoding.EncodeToString
	raw := enc([]byte(`{"alg":"none"}`)) + "." + enc([]byte(`{"sub":"ana","exp":2000000000}`)) + "."

	tok, err := decodificar(raw)
	if err != nil {
		t.Fatalf("decodificar token none: %v", err)
	}
	issues := avaliarClaims(tok.alg(), tok.Payload, time.Unix(1700000000, 0), time.Minute, 24*time.Hour)
	if !temIssue(issues, "alg_none", sevCritical) {
		t.Errorf("alg none deve ser critical: %+v", issues)
	}
	if achou, _ := testarSegredos(tok, []string{"", "x"}); achou {
		t.Error("token none não tem assinatura a validar")
	}
}

func TestExpNoPassadoViraExpirado(t *testing.T) {
	agora := time.Unix(1700000000, 0)
	payload := map[string]any{"exp": float64(agora.Add(-time.Hour).Unix())}

	issues := avaliarClaims("RS256", payload, agora, time.Minute, 24*time.Hour)
	if !temIssue(issues, "expirado", sevInfo) {
		t.Errorf("exp no passado deveria gerar expirado: %+v", issues)
	}
	if got := situacao(payload, agora, time.Minute); got != "expirado" {
		t.Errorf("situacao = %q, quero expirado", got)
	}
}

// A folga existe para o relógio desalinhado não gerar falso "expirado".
func TestLeewayAbsorveExpiracaoRecente(t *testing.T) {
	agora := time.Unix(1700000000, 0)
	payload := map[string]any{"exp": float64(agora.Add(-30 * time.Second).Unix())}

	if got := situacao(payload, agora, time.Minute); got != "valido" {
		t.Errorf("30s após exp com leeway de 60s = %q, quero valido", got)
	}
}

func TestExpAusenteEhMedium(t *testing.T) {
	issues := avaliarClaims("HS256", map[string]any{"sub": "ana"}, time.Now(), time.Minute, 24*time.Hour)
	if !temIssue(issues, "exp_ausente", sevMedium) {
		t.Errorf("sem exp deveria ser medium: %+v", issues)
	}
}

func TestVidaLonga(t *testing.T) {
	agora := time.Unix(1700000000, 0)
	iat := agora.Add(-time.Hour)
	payload := func(vida time.Duration) map[string]any {
		return map[string]any{"iat": float64(iat.Unix()), "exp": float64(iat.Add(vida).Unix())}
	}
	dia := 24 * time.Hour

	if issues := avaliarClaims("RS256", payload(dia), agora, time.Minute, dia); temCheck(issues, "vida_longa") {
		t.Error("exatamente no limite não é vida longa")
	}
	if issues := avaliarClaims("RS256", payload(7*dia), agora, time.Minute, dia); !temIssue(issues, "vida_longa", sevLow) {
		t.Errorf("7 dias deveria ser low: %+v", issues)
	}
	if issues := avaliarClaims("RS256", payload(90*dia), agora, time.Minute, dia); !temIssue(issues, "vida_longa", sevMedium) {
		t.Errorf("90 dias deveria ser medium: %+v", issues)
	}
	if issues := avaliarClaims("RS256", payload(90*dia), agora, time.Minute, 0); temCheck(issues, "vida_longa") {
		t.Error("max-life 0 desliga a checagem")
	}
}

func TestNbfEIatNoFuturo(t *testing.T) {
	agora := time.Unix(1700000000, 0)
	payload := map[string]any{
		"exp": float64(agora.Add(time.Hour).Unix()),
		"nbf": float64(agora.Add(time.Hour).Unix()),
		"iat": float64(agora.Add(time.Hour).Unix()),
	}
	issues := avaliarClaims("RS256", payload, agora, time.Minute, 0)
	if !temCheck(issues, "nbf_futuro") || !temCheck(issues, "iat_futuro") {
		t.Errorf("esperava nbf_futuro e iat_futuro: %+v", issues)
	}
	if got := situacao(payload, agora, time.Minute); got != "ainda_nao_valido" {
		t.Errorf("situacao = %q, quero ainda_nao_valido", got)
	}
}

func TestAlgSimetricoEhInfo(t *testing.T) {
	issues := avaliarClaims("HS256", map[string]any{"exp": float64(2000000000)}, time.Unix(1700000000, 0), time.Minute, 0)
	if !temIssue(issues, "alg_simetrico", sevInfo) {
		t.Errorf("HS256 deveria gerar alg_simetrico info: %+v", issues)
	}
}

// Ponta a ponta: o segredo fraco chega ao registro e o token em si não vaza.
func TestInspecionarReportaSegredoFracoSemVazarToken(t *testing.T) {
	raw := jwtHS256(t, `{"sub":"ana","exp":4000000000}`, "admin")
	cfg := Config{LeewaySec: 60, MaxLifeH: 24}

	reg := inspecionar(raw, cfg, []string{"x", "admin"}, time.Unix(1700000000, 0))
	if reg.WeakSecret != "admin" {
		t.Errorf("WeakSecret = %q, quero admin", reg.WeakSecret)
	}
	if reg.TokenID == "" || strings.Contains(raw, reg.TokenID) {
		t.Errorf("TokenID = %q deve ser um hash, não parte do token", reg.TokenID)
	}
}

func TestInspecionarTokenInvalidoViraRegistroComErro(t *testing.T) {
	reg := inspecionar("lixo", Config{}, nil, time.Now())
	if reg.Error == "" {
		t.Error("token inválido deveria preencher Error")
	}
}

func temCheck(issues []issue, check string) bool {
	for _, i := range issues {
		if i.Check == check {
			return true
		}
	}
	return false
}

func temIssue(issues []issue, check, sev string) bool {
	for _, i := range issues {
		if i.Check == check && i.Severity == sev {
			return true
		}
	}
	return false
}
