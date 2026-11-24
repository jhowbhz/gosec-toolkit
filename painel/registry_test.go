package main

import (
	"strings"
	"testing"
)

func ferramentaTeste() ferramenta {
	return ferramenta{
		ID: "teste",
		Campos: []campo{
			{Flag: "url", Tipo: "texto", Required: true},
			{Flag: "depth", Tipo: "int"},
			{Flag: "scope", Tipo: "enum", Opcoes: []string{"host", "domain"}},
			{Flag: "insecure", Tipo: "bool"},
			{Flag: "header", Tipo: "texto", Repetivel: true},
		},
	}
}

func TestMontarArgvOrdemETipos(t *testing.T) {
	f := ferramentaTeste()
	argv, err := montarArgv(f, map[string][]string{
		"url":      {"https://x.com"},
		"depth":    {"3"},
		"scope":    {"host"},
		"insecure": {"true"},
		"header":   {"A: 1", "B: 2"},
	})
	if err != nil {
		t.Fatalf("montarArgv: %v", err)
	}
	quero := []string{"--url=https://x.com", "--depth=3", "--scope=host", "--insecure", "--header=A: 1", "--header=B: 2"}
	if strings.Join(argv, "\x00") != strings.Join(quero, "\x00") {
		t.Errorf("argv = %v\nquero  = %v", argv, quero)
	}
}

func TestMontarArgvExigeObrigatorio(t *testing.T) {
	_, err := montarArgv(ferramentaTeste(), map[string][]string{"depth": {"1"}})
	if err == nil {
		t.Fatal("faltando --url obrigatório deveria falhar")
	}
}

func TestMontarArgvRejeitaFlagDesconhecida(t *testing.T) {
	_, err := montarArgv(ferramentaTeste(), map[string][]string{
		"url":  {"https://x.com"},
		"evil": {"--rm-rf"},
	})
	if err == nil {
		t.Fatal("flag fora do schema deveria ser rejeitada (segurança)")
	}
}

func TestMontarArgvValidaIntEEnum(t *testing.T) {
	if _, err := montarArgv(ferramentaTeste(), map[string][]string{"url": {"x"}, "depth": {"abc"}}); err == nil {
		t.Error("int inválido deveria falhar")
	}
	if _, err := montarArgv(ferramentaTeste(), map[string][]string{"url": {"x"}, "scope": {"galaxia"}}); err == nil {
		t.Error("enum fora das opções deveria falhar")
	}
}

func TestMontarArgvPosicionalVaiNoFim(t *testing.T) {
	f := ferramenta{ID: "x", Campos: []campo{
		{Flag: "insecure", Tipo: "bool"},
		{Flag: "caminho", Tipo: "run-ref", Posicional: true, Repetivel: true},
	}}
	argv, err := montarArgv(f, map[string][]string{"insecure": {"true"}, "caminho": {"dump/", "b/"}})
	if err != nil {
		t.Fatalf("montarArgv: %v", err)
	}
	quero := []string{"--insecure", "dump/", "b/"}
	if strings.Join(argv, "\x00") != strings.Join(quero, "\x00") {
		t.Errorf("argv = %v, quero %v", argv, quero)
	}
}

func TestMontarArgvBoolDefaultTrueEmiteFalse(t *testing.T) {
	f := ferramenta{Campos: []campo{{Flag: "api-only", Tipo: "bool", Default: "true"}}}
	off, _ := montarArgv(f, map[string][]string{"api-only": {"false"}})
	if len(off) != 1 || off[0] != "--api-only=false" {
		t.Errorf("default-true desligado = %v, quero [--api-only=false]", off)
	}
	on, _ := montarArgv(f, map[string][]string{"api-only": {"true"}})
	if len(on) != 1 || on[0] != "--api-only" {
		t.Errorf("default-true ligado = %v, quero [--api-only]", on)
	}
}

func TestMontarArgvBoolFalseOmite(t *testing.T) {
	argv, err := montarArgv(ferramentaTeste(), map[string][]string{"url": {"x"}, "insecure": {"false"}})
	if err != nil {
		t.Fatalf("montarArgv: %v", err)
	}
	for _, a := range argv {
		if strings.Contains(a, "insecure") {
			t.Errorf("bool false não deveria aparecer no argv: %v", argv)
		}
	}
}
