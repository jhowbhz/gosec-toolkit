package main

import (
	"strings"
	"testing"
)

func TestParseConfigPadroes(t *testing.T) {
	cfg, err := parseConfig([]string{"--old=a.jsonl", "--new=b.jsonl"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Format != "text" {
		t.Errorf("format padrão = %q, quero text", cfg.Format)
	}
	only, err := cfg.onlySet()
	if err != nil {
		t.Fatalf("onlySet: %v", err)
	}
	if len(only) != 3 {
		t.Errorf("--only vazio deve liberar os 3 tipos, veio %v", only)
	}
}

func TestParseConfigOldENewObrigatorios(t *testing.T) {
	casos := map[string][]string{
		"--old": {"--new=b.jsonl"},
		"--new": {"--old=a.jsonl"},
	}
	for flag, args := range casos {
		_, err := parseConfig(args)
		if err == nil {
			t.Fatalf("esperava erro sem %s", flag)
		}
		if !strings.Contains(err.Error(), flag) {
			t.Errorf("erro deveria citar %s: %v", flag, err)
		}
	}
}

func TestParseConfigExtensaoInvalidaRecusada(t *testing.T) {
	_, err := parseConfig([]string{"--old=a.csv", "--new=b.jsonl"})
	if err == nil {
		t.Fatal("esperava erro com extensão .csv")
	}
	if !strings.Contains(err.Error(), "extensão") {
		t.Errorf("mensagem deveria explicar a extensão: %v", err)
	}
}

// validate não pode abrir arquivo: arquivo inexistente com extensão certa tem
// de passar na forma e só falhar no runtime.
func TestParseConfigNaoAbreArquivos(t *testing.T) {
	if _, err := parseConfig([]string{"--old=nao-existe.jsonl", "--new=nao-existe.db"}); err != nil {
		t.Fatalf("validate não deveria tocar o disco: %v", err)
	}
}

func TestParseConfigFormatInvalido(t *testing.T) {
	_, err := parseConfig([]string{"--old=a.jsonl", "--new=b.jsonl", "--format=xml"})
	if err == nil {
		t.Fatal("esperava erro com --format inválido")
	}
}

func TestParseConfigOnlyInvalido(t *testing.T) {
	_, err := parseConfig([]string{"--old=a.jsonl", "--new=b.jsonl", "--only=added,sumiu"})
	if err == nil {
		t.Fatal("esperava erro com item inválido em --only")
	}
}

func TestParseConfigLogFileExigeJSONL(t *testing.T) {
	_, err := parseConfig([]string{"--old=a.jsonl", "--new=b.jsonl", "--log-file=x.jsonl"})
	if err == nil {
		t.Fatal("esperava erro: --log-file com format text seria ignorado em silêncio")
	}
}
