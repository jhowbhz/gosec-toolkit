package main

import (
	"strings"
	"testing"
)

func TestParseConfigPadroes(t *testing.T) {
	cfg, err := parseConfig([]string{"--url=https://exemplo.com.br/api"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Method != "GET" {
		t.Errorf("method padrão = %q, quero GET", cfg.Method)
	}
	if cfg.Rate != 1 {
		t.Errorf("rate padrão = %d, quero 1", cfg.Rate)
	}
	if cfg.Total != 60 {
		t.Errorf("total padrão = %d, quero 60", cfg.Total)
	}
	if cfg.Concurrency != 1 {
		t.Errorf("concurrency padrão = %d, quero 1", cfg.Concurrency)
	}
	if !cfg.StopOn429 {
		t.Error("stop-on-429 deve vir LIGADO por padrão")
	}
	if cfg.Ramp {
		t.Error("ramp deve vir desligada por padrão")
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
	if _, err := parseConfig([]string{"--url=exemplo.com.br"}); err == nil {
		t.Fatal("esperava erro com URL sem scheme")
	}
}

// Os tetos são a garantia de que a ferramenta não vira gerador de carga:
// se algum deixar de ser recusado, ela passa a poder abusar do alvo.
func TestParseConfigRecusaAcimaDosTetos(t *testing.T) {
	casos := []struct {
		nome, flag, quero string
	}{
		{"total", "--total=5001", "--total acima do teto de segurança 5000: 5001"},
		{"rate", "--rate=51", "--rate acima do teto de segurança 50: 51"},
		{"concurrency", "--concurrency=21", "--concurrency acima do teto de segurança 20: 21"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, err := parseConfig([]string{"--url=https://x.com", c.flag})
			if err == nil {
				t.Fatalf("esperava recusa de %s", c.flag)
			}
			if err.Error() != c.quero {
				t.Errorf("erro = %q, quero %q", err.Error(), c.quero)
			}
		})
	}
}

func TestParseConfigAceitaExatamenteOsTetos(t *testing.T) {
	_, err := parseConfig([]string{"--url=https://x.com", "--rate=50", "--total=5000", "--concurrency=20"})
	if err != nil {
		t.Fatalf("os tetos em si devem ser aceitos: %v", err)
	}
}

func TestParseConfigFaixaMinima(t *testing.T) {
	for _, f := range []string{"--rate=0", "--total=0", "--concurrency=0", "--ramp-steps=0", "--timeout=0", "--duration=-1"} {
		if _, err := parseConfig([]string{"--url=https://x.com", f}); err == nil {
			t.Errorf("esperava erro com %s", f)
		}
	}
}

func TestParseConfigContinueOn429DesligaParada(t *testing.T) {
	cfg, err := parseConfig([]string{"--url=https://x.com", "--continue-on-429"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.StopOn429 {
		t.Error("--continue-on-429 deveria desligar o stop-on-429")
	}
}

func TestParseConfigBodyEBodyFileExclusivos(t *testing.T) {
	_, err := parseConfig([]string{"--url=https://x.com", "--body=a", "--body-file=b.json"})
	if err == nil {
		t.Fatal("esperava erro com --body e --body-file juntos")
	}
}

func TestParseConfigHeaderMalformado(t *testing.T) {
	if _, err := parseConfig([]string{"--url=https://x.com", "--header=semdoispontos"}); err == nil {
		t.Fatal("esperava erro com header sem ':'")
	}
}
