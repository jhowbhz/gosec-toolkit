package main

import (
	"strings"
	"testing"
)

func TestParseConfigPadroes(t *testing.T) {
	cfg, err := parseConfig([]string{"--dir=espelho"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if !cfg.Redact {
		t.Error("redact deve vir ligado por padrão")
	}
	if cfg.ContextChars != 40 {
		t.Errorf("context padrão = %d, quero 40", cfg.ContextChars)
	}
	if cfg.TimeoutSec != 15 {
		t.Errorf("timeout padrão = %d, quero 15", cfg.TimeoutSec)
	}
	if cfg.MaxBodyKB != 0 {
		t.Errorf("max-body padrão = %d, quero 0 (sem limite)", cfg.MaxBodyKB)
	}
	for _, ext := range []string{"js", "html", "env", "yaml", "ts"} {
		if !cfg.extensoes()[ext] {
			t.Errorf("extensão %q deveria estar no padrão", ext)
		}
	}
}

func TestParseConfigFonteObrigatoria(t *testing.T) {
	_, err := parseConfig(nil)
	if err == nil {
		t.Fatal("esperava erro sem nenhuma fonte")
	}
	if !strings.Contains(err.Error(), "--dir") {
		t.Errorf("erro deveria citar as fontes aceitas: %v", err)
	}
}

func TestParseConfigQualquerFonteBasta(t *testing.T) {
	for _, arg := range []string{"--dir=a", "--file=a.js", "--url=https://x.com/a.js"} {
		if _, err := parseConfig([]string{arg}); err != nil {
			t.Errorf("%s sozinha deveria bastar: %v", arg, err)
		}
	}
}

func TestParseConfigURLPrecisaDeScheme(t *testing.T) {
	_, err := parseConfig([]string{"--url=exemplo.com.br/app.js"})
	if err == nil {
		t.Fatal("esperava erro com URL sem scheme")
	}
	_, err = parseConfig([]string{"--url=ftp://exemplo.com.br/app.js"})
	if err == nil {
		t.Fatal("esperava erro com scheme que não é http/https")
	}
}

func TestParseConfigHeaderMalformado(t *testing.T) {
	_, err := parseConfig([]string{"--dir=a", "--header=semdoispontos"})
	if err == nil {
		t.Fatal("esperava erro com header sem ':'")
	}
}

func TestParseConfigHeadersViramHTTPHeader(t *testing.T) {
	cfg, err := parseConfig([]string{
		"--url=https://x.com/a.js",
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
}

func TestParseConfigNumerosNegativos(t *testing.T) {
	for _, arg := range []string{"--max-body=-1", "--context=-1", "--timeout=-1"} {
		if _, err := parseConfig([]string{"--dir=a", arg}); err == nil {
			t.Errorf("esperava erro com %s", arg)
		}
	}
}

func TestParseConfigNoRedactDesligaRedacao(t *testing.T) {
	cfg, err := parseConfig([]string{"--dir=a", "--no-redact"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Redact {
		t.Error("--no-redact deveria desligar a redação")
	}
}

func TestParseConfigFlagsRepetiveis(t *testing.T) {
	cfg, err := parseConfig([]string{"--dir=a", "--dir=b", "--file=c.js"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(cfg.Dirs) != 2 || len(cfg.Files) != 1 {
		t.Errorf("dirs=%v files=%v, quero 2 dirs e 1 file", cfg.Dirs, cfg.Files)
	}
}

func TestExtensoesNormalizaPontoECaixa(t *testing.T) {
	cfg := Config{Ext: " .JS, Env ,,json"}
	set := cfg.extensoes()
	for _, e := range []string{"js", "env", "json"} {
		if !set[e] {
			t.Errorf("extensão %q ausente em %v", e, set)
		}
	}
	if len(set) != 3 {
		t.Errorf("len = %d, quero 3 (vazios descartados)", len(set))
	}
}
