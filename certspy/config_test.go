package main

import (
	"strings"
	"testing"
)

func TestParseConfigPadroes(t *testing.T) {
	cfg, err := parseConfig([]string{"--domain=exemplo.com.br"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Port != 443 {
		t.Errorf("port padrão = %d, quero 443", cfg.Port)
	}
	if cfg.Concurrency != 8 {
		t.Errorf("concurrency padrão = %d, quero 8", cfg.Concurrency)
	}
	if cfg.TimeoutS != 20 {
		t.Errorf("timeout padrão = %d, quero 20", cfg.TimeoutS)
	}
	if !cfg.resolverDNS() {
		t.Error("resolução DNS deve vir ligada por padrão")
	}
	if cfg.PublishURL != "" {
		t.Error("publish deve vir desligado por padrão")
	}
	if cfg.UserAgent == "" {
		t.Error("user-agent padrão não pode ser vazio")
	}
}

func TestParseConfigExigeFonte(t *testing.T) {
	_, err := parseConfig(nil)
	if err == nil {
		t.Fatal("esperava erro sem --domain nem --host")
	}
	if !strings.Contains(err.Error(), "--domain") {
		t.Errorf("erro deveria citar --domain: %v", err)
	}
}

func TestParseConfigRecusaNoCTeNoLiveJuntos(t *testing.T) {
	_, err := parseConfig([]string{"--domain=x.com", "--host=x.com", "--no-ct", "--no-live"})
	if err == nil || !strings.Contains(err.Error(), "sem fonte") {
		t.Fatalf("esperava erro 'sem fonte', veio %v", err)
	}
}

func TestParseConfigSoHostSemCTEhValido(t *testing.T) {
	cfg, err := parseConfig([]string{"--host=www.x.com", "--no-ct"})
	if err != nil {
		t.Fatalf("só --host com --no-ct deveria passar: %v", err)
	}
	if cfg.usaCT() || !cfg.usaLive() {
		t.Errorf("fontes erradas: ct=%v live=%v", cfg.usaCT(), cfg.usaLive())
	}
}

func TestParseConfigDomainSemEntradaParaFonteAtiva(t *testing.T) {
	_, err := parseConfig([]string{"--domain=x.com", "--no-ct"})
	if err == nil {
		t.Fatal("--domain com --no-ct e sem --host não tem o que fazer")
	}
}

func TestParseConfigPublishURLPrecisaDeScheme(t *testing.T) {
	_, err := parseConfig([]string{"--domain=x.com", "--publish-url=receptor.exemplo.com/api"})
	if err == nil {
		t.Fatal("esperava erro com --publish-url sem scheme")
	}
	if _, err := parseConfig([]string{"--domain=x.com", "--publish-url=https://receptor.exemplo.com/api"}); err != nil {
		t.Errorf("--publish-url https válida foi recusada: %v", err)
	}
}

func TestParseConfigLimites(t *testing.T) {
	casos := []struct {
		nome string
		arg  string
	}{
		{"port zero", "--port=0"},
		{"port acima", "--port=65536"},
		{"concurrency zero", "--concurrency=0"},
		{"concurrency acima", "--concurrency=65"},
		{"timeout zero", "--timeout=0"},
	}
	for _, c := range casos {
		if _, err := parseConfig([]string{"--domain=x.com", c.arg}); err == nil {
			t.Errorf("%s: esperava erro com %s", c.nome, c.arg)
		}
	}
	for _, ok := range []string{"--port=65535", "--concurrency=64", "--timeout=1"} {
		if _, err := parseConfig([]string{"--domain=x.com", ok}); err != nil {
			t.Errorf("%s deveria passar: %v", ok, err)
		}
	}
}

func TestParseConfigNoResolveVenceResolve(t *testing.T) {
	cfg, err := parseConfig([]string{"--domain=x.com", "--resolve", "--no-resolve"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.resolverDNS() {
		t.Error("--no-resolve deveria vencer")
	}
}

func TestApexesSemDomainDerivaDoHost(t *testing.T) {
	cfg, err := parseConfig([]string{"--host=www.api.alvo.com:8443", "--no-ct"})
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.apexes(); len(got) != 1 || got[0] != "alvo.com" {
		t.Errorf("apexes = %v, quero [alvo.com]", got)
	}
}
