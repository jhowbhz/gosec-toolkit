package main

import (
	"reflect"
	"testing"
	"time"
)

func TestNormalizarNome(t *testing.T) {
	casos := map[string]string{
		"*.Api.Alvo.com":  "api.alvo.com",
		"  WWW.Alvo.com.": "www.alvo.com",
		"*.*.alvo.com":    "alvo.com",
	}
	for in, quero := range casos {
		if got := normalizarNome(in); got != quero {
			t.Errorf("normalizarNome(%q) = %q, quero %q", in, got, quero)
		}
	}
}

func TestSouApex(t *testing.T) {
	apexes := []string{"alvo.com"}
	if !souApex("sub.alvo.com", apexes) || !souApex("alvo.com", apexes) {
		t.Error("sub.alvo.com e o próprio apex deveriam ser aceitos")
	}
	// Guarda o filtro de privacidade: nome de terceiro não pode passar por
	// coincidência de sufixo.
	for _, fora := range []string{"outro.com", "malalvo.com", "alvo.com.evil.net"} {
		if souApex(fora, apexes) {
			t.Errorf("%q não deveria ser aceito como sob alvo.com", fora)
		}
	}
}

func TestMesclarDeduplicaEUneFontes(t *testing.T) {
	got := mesclar([]achado{
		{Nome: "www.alvo.com", Fonte: fonteCT, NotBefore: "2024-05-01T00:00:00"},
		{Nome: "WWW.alvo.com", Fonte: fonteLive},
		{Nome: "www.alvo.com", Fonte: fonteCT, NotBefore: "2023-01-10T00:00:00"},
		{Nome: "api.alvo.com", Fonte: fonteLive},
		{Nome: "dono@alvo.com", Fonte: fonteCT}, // e-mail de cert S/MIME
	})
	if len(got) != 2 {
		t.Fatalf("quero 2 domínios, vieram %d: %+v", len(got), got)
	}
	if got[0].Nome != "api.alvo.com" || got[1].Nome != "www.alvo.com" {
		t.Errorf("fora de ordem: %+v", got)
	}
	www := got[1]
	if !reflect.DeepEqual(www.Fontes, []string{"ct", "live"}) {
		t.Errorf("fontes = %v, quero [ct live]", www.Fontes)
	}
	if www.NotBefore != "2023-01-10T00:00:00" {
		t.Errorf("not_before = %q, quero o mais antigo", www.NotBefore)
	}
}

// Guarda a publicação: sem --publish-all, nada fora do apex pode sair.
func TestMontarPayloadSoSobApexPorPadrao(t *testing.T) {
	doms := mesclar([]achado{
		{Nome: "www.alvo.com", Fonte: fonteCT},
		{Nome: "terceiro.net", Fonte: fonteCT},
	})
	marcarApex(doms, []string{"alvo.com"})
	quando := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	p := montarPayload([]string{"alvo.com"}, doms, false, quando)
	if len(p.Domains) != 1 || p.Domains[0].Nome != "www.alvo.com" {
		t.Errorf("payload padrão = %+v, quero só www.alvo.com", p.Domains)
	}
	if p.GeneratedAt != "2026-01-02T03:04:05Z" {
		t.Errorf("generated_at = %q", p.GeneratedAt)
	}
	if all := montarPayload([]string{"alvo.com"}, doms, true, quando); len(all.Domains) != 2 {
		t.Errorf("--publish-all deveria incluir os 2, veio %d", len(all.Domains))
	}
}

func TestHostPorta(t *testing.T) {
	casos := []struct {
		in    string
		host  string
		porta int
	}{
		{"alvo.com", "alvo.com", 443},
		{"alvo.com:8443", "alvo.com", 8443},
		{"alvo.com:xx", "alvo.com", 443},
	}
	for _, c := range casos {
		h, p := hostPorta(c.in, 443)
		if h != c.host || p != c.porta {
			t.Errorf("hostPorta(%q) = %s,%d quero %s,%d", c.in, h, p, c.host, c.porta)
		}
	}
}
