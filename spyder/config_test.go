package main

import (
	"testing"
	"time"
)

func TestParseConfigMinimo(t *testing.T) {
	cfg, err := parseConfig([]string{"--url=https://site.com"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.URL != "https://site.com" {
		t.Errorf("URL = %q", cfg.URL)
	}
	// defaults sensatos
	if cfg.Scope != scopeSameHost {
		t.Errorf("Scope default = %q, quero same-host", cfg.Scope)
	}
	if cfg.MaxPages < 1 || cfg.MaxDepth < 1 || cfg.Concurrency < 1 {
		t.Errorf("limites default inválidos: pages=%d depth=%d conc=%d", cfg.MaxPages, cfg.MaxDepth, cfg.Concurrency)
	}
}

func TestParseConfigExigeURL(t *testing.T) {
	if _, err := parseConfig([]string{}); err == nil {
		t.Fatal("quero erro quando --url falta")
	}
}

func TestParseConfigURLPrecisaDeScheme(t *testing.T) {
	if _, err := parseConfig([]string{"--url=site.com"}); err == nil {
		t.Fatal("quero erro quando --url não tem http/https")
	}
}

func TestParseConfigScopeInvalido(t *testing.T) {
	if _, err := parseConfig([]string{"--url=https://x.com", "--scope=tudo"}); err == nil {
		t.Fatal("quero erro com scope inválido")
	}
}

func TestParseConfigHeaderEDelay(t *testing.T) {
	cfg, err := parseConfig([]string{"--url=https://x.com", "--header", "Authorization: Bearer z", "--delay=500ms"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(cfg.Headers) != 1 || cfg.Headers[0].Name != "Authorization" {
		t.Errorf("Headers = %+v", cfg.Headers)
	}
	if cfg.Delay != 500*time.Millisecond {
		t.Errorf("Delay = %v, quero 500ms", cfg.Delay)
	}
}

func TestSeedURL(t *testing.T) {
	cfg, _ := parseConfig([]string{"--url=https://site.com/base"})
	u, err := cfg.seedURL()
	if err != nil {
		t.Fatalf("seedURL: %v", err)
	}
	if u.Host != "site.com" {
		t.Errorf("seed host = %q", u.Host)
	}
}
