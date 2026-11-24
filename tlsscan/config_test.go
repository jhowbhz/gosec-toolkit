package main

import (
	"strings"
	"testing"
)

func TestParseConfigPadroes(t *testing.T) {
	cfg, err := parseConfig([]string{"--host=exemplo.com.br"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Port != 443 {
		t.Errorf("port padrão = %d, quero 443", cfg.Port)
	}
	if cfg.TimeoutS != 15 {
		t.Errorf("timeout padrão = %d, quero 15", cfg.TimeoutS)
	}
	if cfg.FullChain || cfg.NoHSTS {
		t.Error("full-chain e no-hsts devem vir desligados por padrão")
	}
}

func TestParseConfigHostObrigatorio(t *testing.T) {
	_, err := parseConfig(nil)
	if err == nil {
		t.Fatal("esperava erro sem --host nem --hosts-file")
	}
	if !strings.Contains(err.Error(), "--host") {
		t.Errorf("erro deveria citar --host: %v", err)
	}
}

func TestParseConfigHostsFileBasta(t *testing.T) {
	if _, err := parseConfig([]string{"--hosts-file=alvos.txt"}); err != nil {
		t.Fatalf("--hosts-file sozinho deveria bastar: %v", err)
	}
}

func TestParseConfigHostRepetivel(t *testing.T) {
	cfg, err := parseConfig([]string{"--host=a.com", "--host=b.com:8443"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(cfg.Hosts) != 2 {
		t.Errorf("hosts = %v, quero 2 itens", cfg.Hosts)
	}
}

func TestParseConfigPortaInvalida(t *testing.T) {
	for _, p := range []string{"0", "65536", "-1"} {
		if _, err := parseConfig([]string{"--host=x.com", "--port=" + p}); err == nil {
			t.Errorf("esperava erro com --port=%s", p)
		}
	}
}

func TestParseConfigTimeoutMinimo(t *testing.T) {
	if _, err := parseConfig([]string{"--host=x.com", "--timeout=0"}); err == nil {
		t.Fatal("esperava erro com timeout < 1")
	}
}

func TestSepararAlvoAplicaPortaPadrao(t *testing.T) {
	if h, p := separarAlvo("x.com", 443); h != "x.com" || p != "443" {
		t.Errorf("sem porta: %s %s", h, p)
	}
	if h, p := separarAlvo("x.com:8443", 443); h != "x.com" || p != "8443" {
		t.Errorf("com porta: %s %s", h, p)
	}
}
