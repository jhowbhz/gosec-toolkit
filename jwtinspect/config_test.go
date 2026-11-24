package main

import (
	"strings"
	"testing"
)

func TestParseConfigPadroes(t *testing.T) {
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatalf("parseConfig sem args deve passar (token pode vir do stdin): %v", err)
	}
	if cfg.LeewaySec != 60 {
		t.Errorf("leeway padrão = %d, quero 60", cfg.LeewaySec)
	}
	if cfg.MaxLifeH != 24 {
		t.Errorf("max-life padrão = %d, quero 24", cfg.MaxLifeH)
	}
	if cfg.Quiet {
		t.Error("quiet deve vir desligado por padrão")
	}
}

func TestParseConfigLeewayNegativo(t *testing.T) {
	_, err := parseConfig([]string{"--leeway=-1"})
	if err == nil {
		t.Fatal("esperava erro com leeway negativo")
	}
	if !strings.Contains(err.Error(), "--leeway") {
		t.Errorf("erro deveria citar --leeway: %v", err)
	}
}

func TestParseConfigMaxLifeNegativo(t *testing.T) {
	_, err := parseConfig([]string{"--max-life=-5"})
	if err == nil {
		t.Fatal("esperava erro com max-life negativo")
	}
}

func TestParseConfigCaminhoVazio(t *testing.T) {
	for _, flag := range []string{"--token-file=", "--secrets="} {
		if _, err := parseConfig([]string{flag}); err == nil {
			t.Errorf("esperava erro com %s sem caminho", flag)
		}
	}
}

func TestParseConfigRepetiveis(t *testing.T) {
	cfg, err := parseConfig([]string{"--token=a.b.c", "--token=d.e.f", "--secret=um", "--secret=dois"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(cfg.Tokens) != 2 || len(cfg.Secrets) != 2 {
		t.Errorf("tokens=%v secrets=%v, quero 2 de cada", cfg.Tokens, cfg.Secrets)
	}
}
