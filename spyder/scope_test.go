package main

import (
	"net/url"
	"testing"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", s, err)
	}
	return u
}

func TestNormalizarResolveRelativoETiraFragmento(t *testing.T) {
	base := mustURL(t, "https://site.com/a/b")
	u, ok := normalizar(base, "../c?x=1#secao")
	if !ok {
		t.Fatal("deveria normalizar")
	}
	if got := u.String(); got != "https://site.com/c?x=1" {
		t.Errorf("normalizar = %q, quero https://site.com/c?x=1 (relativo resolvido, fragmento removido)", got)
	}
}

func TestNormalizarIgnoraNaoHTTP(t *testing.T) {
	base := mustURL(t, "https://site.com/")
	for _, ref := range []string{"mailto:x@y.com", "javascript:void(0)", "tel:123"} {
		if _, ok := normalizar(base, ref); ok {
			t.Errorf("%q não deveria virar rota http", ref)
		}
	}
}

func TestDentroDoEscopoMesmoHost(t *testing.T) {
	seed := mustURL(t, "https://site.com/")
	if !dentroDoEscopo(mustURL(t, "https://site.com/x"), seed, "same-host") {
		t.Error("mesmo host deveria estar no escopo")
	}
	if dentroDoEscopo(mustURL(t, "https://api.site.com/x"), seed, "same-host") {
		t.Error("subdomínio NÃO entra em same-host")
	}
}

func TestDentroDoEscopoSubdominios(t *testing.T) {
	seed := mustURL(t, "https://site.com/")
	if !dentroDoEscopo(mustURL(t, "https://api.site.com/x"), seed, "subdomains") {
		t.Error("subdomínio deveria entrar em scope=subdomains")
	}
	if dentroDoEscopo(mustURL(t, "https://outro.com/x"), seed, "subdomains") {
		t.Error("domínio externo não entra nem em subdomains")
	}
}

func TestForaDoEscopoExterno(t *testing.T) {
	seed := mustURL(t, "https://site.com/")
	if dentroDoEscopo(mustURL(t, "https://vercel.app/x"), seed, "same-host") {
		t.Error("externo não deveria estar no escopo")
	}
}
