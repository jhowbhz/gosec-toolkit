package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

// regraPorNome acha a regra no catálogo; falha o teste se ela sumiu, porque
// remover uma regra por engano é exatamente a regressão que isto guarda.
func regraPorNome(t *testing.T, nome string) regra {
	t.Helper()
	for _, r := range catalogo() {
		if r.Nome == nome {
			return r
		}
	}
	t.Fatalf("regra %q não está no catálogo", nome)
	return regra{}
}

func jwtFalso(payload string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + enc([]byte(payload)) + "." + enc([]byte("assinatura"))
}

// TestCatalogoCasaPositivoENaoCasaNegativo guarda a promessa central: cada
// regra reconhece o formato conhecido do segredo e ignora texto comum. Se
// uma regra afrouxar, o relatório vira ruído; se apertar, segredo passa.
func TestCatalogoCasaPositivoENaoCasaNegativo(t *testing.T) {
	casos := []struct {
		regra, positivo, negativo string
	}{
		{"aws-access-key", `key="AKIAIOSFODNN7EXAMPLE"`, `id="AKIA123"`},
		{"google-api-key", `k: "AIzaSyA1234567890abcdefghijklmnopqrstuv"`, `AIza curto`},
		{"github-token", `ghp_abcdefghijklmnopqrstuvwxyz0123456789`, `ghp_curto`},
		{"slack-token", `xoxb-123456789012-abcdefghij`, `xox-abc`},
		{"jwt", jwtFalso(`{"sub":"1"}`), `eyJhbGciOi.naoeumjwt`},
		{"private-key", `-----BEGIN RSA PRIVATE KEY-----`, `-----BEGIN PUBLIC KEY-----`},
		{"bearer-token", `Authorization: Bearer abcdefghijklmnopqrstuvwxyz012345`, `Bearer abc`},
		{"connection-string", `postgres://admin:hunter2pass@db.interno:5432/app`, `postgres://localhost:5432/app`},
		{"url-basic-auth", `https://usuario:senha123@exemplo.com.br/`, `https://exemplo.com.br/caminho`},
		{"generic-secret", `api_key = "abcd1234efgh5678"`, `password: abc`},
		{"supabase-key", `sb_publishable_abcdefghijklmnopqrstu`, `sbx_curto`},
		{"supabase-service-role", jwtFalso(`{"iss":"supabase","role":"service_role"}`), jwtFalso(`{"iss":"supabase","role":"anon"}`)},
	}
	vistas := map[string]bool{}
	for _, c := range casos {
		vistas[c.regra] = true
		r := regraPorNome(t, c.regra)
		if !r.Re.MatchString(c.positivo) {
			t.Errorf("%s deveria casar %q", c.regra, c.positivo)
		}
		if r.Re.MatchString(c.negativo) {
			t.Errorf("%s NÃO deveria casar %q", c.regra, c.negativo)
		}
	}
	for _, r := range catalogo() {
		if !vistas[r.Nome] {
			t.Errorf("regra %q do catálogo não tem caso de teste", r.Nome)
		}
	}
}

// TestServiceRoleEmQualquerAlinhamento: o "service_role" vem em base64 e a
// forma codificada muda conforme os bytes que o antecedem no payload.
func TestServiceRoleEmQualquerAlinhamento(t *testing.T) {
	r := regraPorNome(t, "supabase-service-role")
	for _, prefixo := range []string{"", "x", "xx", `{"a":"b",`} {
		payload := prefixo + `"role":"service_role"}`
		if !strings.HasPrefix(payload, "{") {
			payload = "{" + payload
		}
		if tok := jwtFalso(payload); !r.Re.MatchString(tok) {
			t.Errorf("não casou service_role com payload %q", payload)
		}
	}
}

func TestCatalogoEstruturaValida(t *testing.T) {
	validas := map[string]bool{}
	for _, s := range ordemSeveridades {
		validas[s] = true
	}
	nomes := map[string]bool{}
	for _, r := range catalogo() {
		if nomes[r.Nome] {
			t.Errorf("nome de regra duplicado: %q", r.Nome)
		}
		nomes[r.Nome] = true
		if !validas[r.Severidade] {
			t.Errorf("%s: severidade inválida %q", r.Nome, r.Severidade)
		}
		if r.Nota == "" {
			t.Errorf("%s: falta a nota em pt-BR", r.Nome)
		}
	}
}

func TestRedigirMascaraEPreservaPrefixo(t *testing.T) {
	segredo := "AKIAIOSFODNN7EXAMPLE" // 20 chars
	got := redigir(segredo)
	if !strings.HasPrefix(got, "AKIA") {
		t.Errorf("redigir(%q) = %q, deveria preservar o prefixo AKIA", segredo, got)
	}
	if strings.Contains(got, "IOSFODNN7EXAMPLE") || strings.Contains(got, "OSFOD") {
		t.Errorf("redigir vazou o miolo do segredo: %q", got)
	}
	if want := "AKIA…[16]"; got != want {
		t.Errorf("redigir = %q, quero %q", got, want)
	}
}

func TestRedigirSegredoCurtoNaoMostraMetade(t *testing.T) {
	got := redigir("abcdefgh") // 8 chars: mostrar 4 seria metade
	if strings.Contains(got, "abcd") {
		t.Errorf("segredo curto expôs metade: %q", got)
	}
	if got := redigir("ab"); strings.Contains(got, "a") {
		t.Errorf("segredo de 2 chars deveria sumir inteiro: %q", got)
	}
}

func TestRedigirRespeitaUTF8(t *testing.T) {
	got := redigir("çççççççççççç")
	if !strings.HasPrefix(got, "çççç…") {
		t.Errorf("redigir cortou rune ao meio: %q", got)
	}
}

// TestAnalisarLinhaNuncaVazaSegredoRedigido guarda o ponto do --redact: nem
// o match, nem o contexto, nem um segredo vizinho na mesma linha podem sair
// inteiros.
func TestAnalisarLinhaNuncaVazaSegredoRedigido(t *testing.T) {
	aws := "AKIAIOSFODNN7EXAMPLE"
	gh := "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	linha := `var a="` + aws + `",b="` + gh + `";`

	achados := analisarLinha(linha, catalogo(), 40, true)
	if len(achados) < 2 {
		t.Fatalf("esperava >= 2 achados, veio %d", len(achados))
	}
	for _, a := range achados {
		for _, texto := range []string{a.Match, a.Contexto} {
			if strings.Contains(texto, aws[4:]) || strings.Contains(texto, gh[8:]) {
				t.Errorf("regra %s vazou segredo: %q", a.Regra.Nome, texto)
			}
		}
	}
}

func TestAnalisarLinhaSemRedacaoMostraTudo(t *testing.T) {
	aws := "AKIAIOSFODNN7EXAMPLE"
	achados := analisarLinha(`k="`+aws+`"`, catalogo(), 40, false)
	if len(achados) != 1 {
		t.Fatalf("esperava 1 achado, veio %d", len(achados))
	}
	if achados[0].Match != aws {
		t.Errorf("match = %q, quero %q", achados[0].Match, aws)
	}
	if !strings.Contains(achados[0].Contexto, aws) {
		t.Errorf("contexto deveria ter o segredo com --no-redact: %q", achados[0].Contexto)
	}
}

func TestAnalisarLinhaLimitaContexto(t *testing.T) {
	// espaços ao redor: a regra AWS exige fronteira de palavra
	linha := strings.Repeat("x", 200) + " AKIAIOSFODNN7EXAMPLE " + strings.Repeat("y", 200)
	achados := analisarLinha(linha, catalogo(), 10, false)
	if len(achados) != 1 {
		t.Fatalf("esperava 1 achado, veio %d", len(achados))
	}
	if got, max := len(achados[0].Contexto), 20+2*10; got > max {
		t.Errorf("contexto com %d bytes, máximo %d", got, max)
	}
}

func TestAnalisarLinhaSemSegredo(t *testing.T) {
	if got := analisarLinha(`console.log("olá mundo")`, catalogo(), 40, true); len(got) != 0 {
		t.Errorf("linha comum não deveria ter achados: %+v", got)
	}
}
