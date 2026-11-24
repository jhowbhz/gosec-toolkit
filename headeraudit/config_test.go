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
	if cfg.Origin != "https://attacker.example" {
		t.Errorf("origin padrão = %q, quero https://attacker.example", cfg.Origin)
	}
	if cfg.Concurrency != 4 {
		t.Errorf("concurrency padrão = %d, quero 4", cfg.Concurrency)
	}
	if cfg.TimeoutS != 15 {
		t.Errorf("timeout padrão = %d, quero 15", cfg.TimeoutS)
	}
	if cfg.DelayMS != 0 {
		t.Errorf("delay padrão = %d, quero 0", cfg.DelayMS)
	}
	if cfg.Insecure {
		t.Error("insecure deve vir desligado por padrão")
	}
}

func TestParseConfigFonteObrigatoria(t *testing.T) {
	_, err := parseConfig(nil)
	if err == nil {
		t.Fatal("esperava erro sem nenhuma fonte de URL")
	}
	if !strings.Contains(err.Error(), "--url") {
		t.Errorf("erro deveria citar --url: %v", err)
	}
}

func TestParseConfigAceitaQualquerFonte(t *testing.T) {
	for _, arg := range []string{"--url=https://x.com", "--urls-file=urls.txt", "--jsonl=m.jsonl"} {
		if _, err := parseConfig([]string{arg}); err != nil {
			t.Errorf("%s deveria bastar como fonte: %v", arg, err)
		}
	}
}

func TestParseConfigNaoLeArquivoNoValidate(t *testing.T) {
	// validate é só forma: arquivo inexistente só falha no runtime.
	if _, err := parseConfig([]string{"--urls-file=/nao/existe.txt"}); err != nil {
		t.Fatalf("validate não deveria tocar o disco: %v", err)
	}
}

func TestParseConfigURLPrecisaDeScheme(t *testing.T) {
	_, err := parseConfig([]string{"--url=exemplo.com.br"})
	if err == nil {
		t.Fatal("esperava erro com URL sem scheme")
	}
}

func TestParseConfigURLRepetivel(t *testing.T) {
	cfg, err := parseConfig([]string{"--url=https://a.com", "--url=http://b.com/x"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(cfg.URLs) != 2 {
		t.Errorf("URLs = %v, quero 2 itens", cfg.URLs)
	}
}

func TestParseConfigOriginInvalido(t *testing.T) {
	for _, o := range []string{"attacker.example", "https://a.com/caminho"} {
		if _, err := parseConfig([]string{"--url=https://x.com", "--origin=" + o}); err == nil {
			t.Errorf("esperava erro com --origin=%s", o)
		}
	}
}

func TestParseConfigNumerosIncoerentes(t *testing.T) {
	for _, arg := range []string{"--concurrency=0", "--delay=-1", "--timeout=0"} {
		if _, err := parseConfig([]string{"--url=https://x.com", arg}); err == nil {
			t.Errorf("esperava erro com %s", arg)
		}
	}
}

func TestParseConfigHeaderMalformado(t *testing.T) {
	_, err := parseConfig([]string{"--url=https://x.com", "--header=semdoispontos"})
	if err == nil {
		t.Fatal("esperava erro com header sem ':'")
	}
}

func TestParseConfigHeadersViramHTTPHeader(t *testing.T) {
	cfg, err := parseConfig([]string{"--url=https://x.com", "--header=Cookie: sid=abc"})
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
