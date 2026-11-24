package main

import (
	"strings"
	"testing"
)

func TestParseConfigPadroes(t *testing.T) {
	cfg, err := parseConfig([]string{"--url=https://exemplo.com.br/app"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Scope != "host" {
		t.Errorf("scope padrão = %q, quero host", cfg.Scope)
	}
	if cfg.Concurrency != 4 {
		t.Errorf("concurrency padrão = %d, quero 4", cfg.Concurrency)
	}
	if cfg.Depth != -1 {
		t.Errorf("depth padrão = %d, quero -1 (ilimitado)", cfg.Depth)
	}
	if cfg.UserAgent == "" {
		t.Error("user-agent padrão não pode ser vazio")
	}
	if cfg.ProbeSensitive {
		t.Error("probe-sensitive deve vir desligado por padrão")
	}
}

func TestParseConfigURLObrigatoria(t *testing.T) {
	_, err := parseConfig(nil)
	if err == nil {
		t.Fatal("esperava erro sem --url")
	}
	if !strings.Contains(err.Error(), "--url") {
		t.Errorf("erro deveria citar --url: %v", err)
	}
}

func TestParseConfigURLPrecisaDeScheme(t *testing.T) {
	_, err := parseConfig([]string{"--url=exemplo.com.br"})
	if err == nil {
		t.Fatal("esperava erro com URL sem scheme")
	}
}

func TestParseConfigScopeInvalido(t *testing.T) {
	_, err := parseConfig([]string{"--url=https://x.com", "--scope=galaxia"})
	if err == nil {
		t.Fatal("esperava erro com scope inválido")
	}
}

func TestParseConfigHeaderMalformado(t *testing.T) {
	_, err := parseConfig([]string{"--url=https://x.com", "--header=semdoispontos"})
	if err == nil {
		t.Fatal("esperava erro com header sem ':'")
	}
}

func TestParseConfigHeadersViramHTTPHeader(t *testing.T) {
	cfg, err := parseConfig([]string{
		"--url=https://x.com",
		"--header=Cookie: sid=abc",
		"--header=X-Teste: 1",
	})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	h, err := cfg.parsedHeaders()
	if err != nil {
		t.Fatalf("parsedHeaders: %v", err)
	}
	if got := h.Get("Cookie"); got != "sid=abc" {
		t.Errorf("Cookie = %q, quero sid=abc", got)
	}
	if got := h.Get("X-Teste"); got != "1" {
		t.Errorf("X-Teste = %q, quero 1", got)
	}
}

func TestParseConfigIncludeRegexInvalida(t *testing.T) {
	_, err := parseConfig([]string{"--url=https://x.com", "--include=(abc"})
	if err == nil {
		t.Fatal("esperava erro com regex de --include inválida")
	}
}

func TestParseConfigConcorrenciaMinima(t *testing.T) {
	_, err := parseConfig([]string{"--url=https://x.com", "--concurrency=0"})
	if err == nil {
		t.Fatal("esperava erro com concurrency < 1")
	}
}
